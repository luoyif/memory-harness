package plugins

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	maxPackageBytes = 32 << 20
	maxEntryBytes   = 4 << 20
	maxEntries      = 256
)

type parsedPackage struct {
	Manifest    Manifest
	ManifestRaw []byte
	Signature   Signature
	Files       map[string][]byte
	Digest      []byte
	Hash        string
}

func safePackagePath(name string) bool {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.ContainsRune(name, '\x00') {
		return false
	}
	cleaned := path.Clean(name)
	return cleaned == name && cleaned != "." && cleaned != ".." && !strings.HasPrefix(cleaned, "../")
}

func declaredRoot(name string) bool {
	if name == "manifest.yaml" || name == "SIGNATURE" {
		return true
	}
	for _, prefix := range []string{"schemas/", "pipelines/", "blueprints/", "prompts/", "ui/", "wasm/", "docs/"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func parsePackage(raw []byte) (parsedPackage, error) {
	if len(raw) == 0 || len(raw) > maxPackageBytes {
		return parsedPackage{}, fmt.Errorf("plugin package must be between 1 and %d bytes", maxPackageBytes)
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return parsedPackage{}, err
	}
	if len(reader.File) == 0 || len(reader.File) > maxEntries {
		return parsedPackage{}, fmt.Errorf("plugin package entry count must be between 1 and %d", maxEntries)
	}
	files := map[string][]byte{}
	var total int64
	for _, entry := range reader.File {
		if !safePackagePath(entry.Name) || !declaredRoot(entry.Name) {
			return parsedPackage{}, fmt.Errorf("unsafe or undeclared plugin entry %q", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.Mode()&0o170000 != 0 && !entry.Mode().IsRegular() {
			return parsedPackage{}, fmt.Errorf("plugin entry %q is not a regular file", entry.Name)
		}
		if entry.UncompressedSize64 > maxEntryBytes {
			return parsedPackage{}, fmt.Errorf("plugin entry %q exceeds size limit", entry.Name)
		}
		if _, exists := files[entry.Name]; exists {
			return parsedPackage{}, fmt.Errorf("duplicate plugin entry %q", entry.Name)
		}
		stream, err := entry.Open()
		if err != nil {
			return parsedPackage{}, err
		}
		data, readErr := io.ReadAll(io.LimitReader(stream, maxEntryBytes+1))
		closeErr := stream.Close()
		if readErr != nil {
			return parsedPackage{}, readErr
		}
		if closeErr != nil {
			return parsedPackage{}, closeErr
		}
		if len(data) > maxEntryBytes {
			return parsedPackage{}, fmt.Errorf("plugin entry %q exceeds size limit", entry.Name)
		}
		total += int64(len(data))
		if total > maxPackageBytes {
			return parsedPackage{}, errors.New("expanded plugin package exceeds size limit")
		}
		files[entry.Name] = data
	}
	manifestRaw, ok := files["manifest.yaml"]
	if !ok {
		return parsedPackage{}, errors.New("manifest.yaml is required")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(manifestRaw))
	decoder.KnownFields(true)
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return parsedPackage{}, fmt.Errorf("manifest: %w", err)
	}
	var signature Signature
	if signatureRaw, ok := files["SIGNATURE"]; ok {
		if err := decodeStrictJSON(signatureRaw, &signature); err != nil {
			return parsedPackage{}, fmt.Errorf("SIGNATURE: %w", err)
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		if name != "SIGNATURE" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	digest := sha256.New()
	for _, name := range names {
		digest.Write([]byte(name))
		digest.Write([]byte{0})
		digest.Write(files[name])
		digest.Write([]byte{0})
	}
	digestBytes := digest.Sum(nil)
	return parsedPackage{Manifest: manifest, ManifestRaw: manifestRaw, Signature: signature, Files: files, Digest: digestBytes, Hash: "sha256:" + hex.EncodeToString(digestBytes)}, nil
}
