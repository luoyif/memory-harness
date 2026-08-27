package exporter

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/app"
)

type FileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	SchemaVersion string      `json:"schema_version"`
	ExportedAt    string      `json:"exported_at"`
	LedgerRecords int         `json:"ledger_records"`
	Includes      []string    `json:"includes"`
	Files         []FileEntry `json:"files"`
}

type sourceFile struct {
	abs, rel string
	info     os.FileInfo
	sha      string
}

func Create(ctx context.Context, a *app.App, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "memoryos-export-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	snapshot := filepath.Join(tempDir, "control.sqlite")
	escaped := strings.ReplaceAll(snapshot, "'", "''")
	if _, err := a.Control.DB.ExecContext(ctx, `VACUUM INTO '`+escaped+`'`); err != nil {
		return err
	}
	files, err := collect(ctx, a, filepath.Join(tempDir, "files"))
	if err != nil {
		return err
	}
	info, err := os.Stat(snapshot)
	if err != nil {
		return err
	}
	hash, err := hashFile(snapshot)
	if err != nil {
		return err
	}
	files = append(files, sourceFile{abs: snapshot, rel: "state/control.sqlite", info: info, sha: hash})
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	count, err := snapshotReceiptCount(snapshot)
	if err != nil {
		return err
	}
	ledgerLines, err := snapshotLedgerLines(files)
	if err != nil {
		return err
	}
	if ledgerLines != count {
		return fmt.Errorf("export snapshot changed during capture: ledger lines=%d control receipts=%d; retry export", ledgerLines, count)
	}
	entries := make([]FileEntry, 0, len(files))
	for _, f := range files {
		entries = append(entries, FileEntry{Path: f.rel, Size: f.info.Size(), SHA256: f.sha})
	}
	mf := Manifest{SchemaVersion: "0.3", ExportedAt: time.Now().UTC().Format(time.RFC3339Nano), LedgerRecords: count, Includes: []string{"ledger", "memory", "sources", "control-snapshot"}, Files: entries}
	mb, _ := json.MarshalIndent(mf, "", "  ")

	outFile, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(outFile)
	tw := tar.NewWriter(gz)
	fail := func(e error) error { _ = tw.Close(); _ = gz.Close(); _ = outFile.Close(); return e }
	if err := writeBytes(tw, "export-manifest.json", mb, 0o600); err != nil {
		return fail(err)
	}
	for _, src := range files {
		in, err := os.Open(src.abs)
		if err != nil {
			return fail(err)
		}
		h := &tar.Header{Name: src.rel, Mode: 0600, Size: src.info.Size(), ModTime: src.info.ModTime()}
		if err := tw.WriteHeader(h); err != nil {
			in.Close()
			return fail(err)
		}
		_, copyErr := io.Copy(tw, in)
		closeErr := in.Close()
		if copyErr != nil {
			return fail(copyErr)
		}
		if closeErr != nil {
			return fail(closeErr)
		}
	}
	if err := tw.Close(); err != nil {
		return fail(err)
	}
	if err := gz.Close(); err != nil {
		_ = outFile.Close()
		return err
	}
	if err := outFile.Sync(); err != nil {
		_ = outFile.Close()
		return err
	}
	return outFile.Close()
}

func collect(ctx context.Context, a *app.App, snapshotRoot string) ([]sourceFile, error) {
	out := []sourceFile{}
	for _, base := range []string{"ledger", "memory", "sources"} {
		root := filepath.Join(a.Config.Home, base)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("export refuses non-regular file %s", path)
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			rel, err := filepath.Rel(a.Config.Home, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			destination := filepath.Join(snapshotRoot, filepath.FromSlash(rel))
			if err := copySnapshot(path, destination); err != nil {
				return err
			}
			snapshotInfo, err := os.Stat(destination)
			if err != nil {
				return err
			}
			h, err := hashFile(destination)
			if err != nil {
				return err
			}
			out = append(out, sourceFile{abs: destination, rel: strings.TrimPrefix(rel, "/"), info: snapshotInfo, sha: h})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out, nil
}

func copySnapshot(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func snapshotReceiptCount(path string) (int, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var count int
	err = db.QueryRow(`SELECT count(*) FROM evidence_receipts`).Scan(&count)
	return count, err
}

func snapshotLedgerLines(files []sourceFile) (int, error) {
	count := 0
	for _, file := range files {
		if !strings.HasPrefix(file.rel, "ledger/") || !strings.HasSuffix(file.rel, ".jsonl") {
			continue
		}
		in, err := os.Open(file.abs)
		if err != nil {
			return 0, err
		}
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 64<<10), 32<<20)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) != "" {
				count++
			}
		}
		scanErr := scanner.Err()
		closeErr := in.Close()
		if scanErr != nil {
			return 0, scanErr
		}
		if closeErr != nil {
			return 0, closeErr
		}
	}
	return count, nil
}
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func writeBytes(tw *tar.Writer, name string, b []byte, mode int64) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(b)), ModTime: time.Now()}); err != nil {
		return err
	}
	_, err := tw.Write(b)
	return err
}
