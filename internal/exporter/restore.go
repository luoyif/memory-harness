package exporter

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Restore(input, targetHome string) error {
	input, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	targetHome, err = filepath.Abs(targetHome)
	if err != nil {
		return err
	}
	if targetHome == string(filepath.Separator) {
		return errors.New("refusing filesystem root as restore target")
	}
	if entries, readErr := os.ReadDir(targetHome); readErr == nil && len(entries) > 0 {
		return errors.New("restore target must be empty")
	} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	parent := filepath.Dir(targetHome)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".memoryos-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var manifest Manifest
	seenManifest := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Clean(header.Name))
		if name == "." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
			return fmt.Errorf("unsafe archive path %q", header.Name)
		}
		allowed := name == "export-manifest.json" || name == "state/control.sqlite" || strings.HasPrefix(name, "ledger/") || strings.HasPrefix(name, "memory/") || strings.HasPrefix(name, "sources/")
		if !allowed || header.Typeflag != tar.TypeReg {
			return fmt.Errorf("unsupported archive entry %q", name)
		}
		if header.Size < 0 || header.Size > 8<<30 {
			return fmt.Errorf("invalid archive entry size for %q", name)
		}
		if name == "export-manifest.json" {
			if err := json.NewDecoder(io.LimitReader(tr, 4<<20)).Decode(&manifest); err != nil {
				return err
			}
			seenManifest = true
			continue
		}
		destination := filepath.Join(stage, filepath.FromSlash(name))
		if !strings.HasPrefix(destination, stage+string(filepath.Separator)) {
			return fmt.Errorf("unsafe archive destination %q", name)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return err
		}
		out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(out, tr, header.Size)
		syncErr := out.Sync()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if !seenManifest || manifest.SchemaVersion != "0.3" {
		return errors.New("missing or unsupported export manifest")
	}
	if len(manifest.Files) == 0 {
		return errors.New("export manifest contains no files")
	}
	for _, entry := range manifest.Files {
		path := filepath.Join(stage, filepath.FromSlash(entry.Path))
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("manifest file %s: %w", entry.Path, err)
		}
		if info.Size() != entry.Size {
			return fmt.Errorf("size mismatch for %s", entry.Path)
		}
		hash, err := hashFile(path)
		if err != nil {
			return err
		}
		if hash != entry.SHA256 {
			return fmt.Errorf("checksum mismatch for %s", entry.Path)
		}
	}
	if _, err := os.Stat(filepath.Join(stage, "state", "control.sqlite")); err != nil {
		return errors.New("control snapshot is missing")
	}
	if err := os.Remove(targetHome); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(stage, targetHome); err != nil {
		return err
	}
	return nil
}
