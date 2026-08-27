package sourcearchive

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/luoyif/memory-harness/internal/contracts"
)

type Result struct {
	Hash    string `json:"sha256"`
	RelPath string `json:"relative_path"`
	Size    int    `json:"size"`
}

func Preserve(root string, raw []byte) (Result, error) {
	hash := contracts.HashBytes(raw)
	rel := filepath.ToSlash(filepath.Join("imports", hash[:2], hash+".json"))
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return Result{}, err
	}
	if existing, err := os.ReadFile(abs); err == nil {
		if contracts.HashBytes(existing) != hash {
			return Result{}, errors.New("source archive integrity conflict")
		}
		return Result{Hash: hash, RelPath: filepath.ToSlash(filepath.Join("sources", rel)), Size: len(raw)}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}
	temp, err := os.CreateTemp(filepath.Dir(abs), ".source-")
	if err != nil {
		return Result{}, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return Result{}, err
	}
	if _, err := temp.Write(raw); err != nil {
		temp.Close()
		return Result{}, err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return Result{}, err
	}
	if err := temp.Close(); err != nil {
		return Result{}, err
	}
	if err := os.Rename(tempName, abs); err != nil {
		return Result{}, err
	}
	return Result{Hash: hash, RelPath: filepath.ToSlash(filepath.Join("sources", rel)), Size: len(raw)}, nil
}
