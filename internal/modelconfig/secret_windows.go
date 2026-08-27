//go:build windows

package modelconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	credentialTypeGeneric  = 1
	credentialPersistLocal = 2
	credentialMaxBlobSize  = 2560
)

var (
	advapi32        = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

// windowsCredential mirrors CREDENTIALW from wincred.h. Pointer-sized fields
// are kept as pointers so the layout remains correct on both amd64 and arm64.
type windowsCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type windowsCredentialStore struct{ namespace string }

func NewDefaultSecretStore(home string) SecretStore {
	digest := sha256.Sum256([]byte(home))
	return &windowsCredentialStore{namespace: "MemoryOS.ModelProviders." + hex.EncodeToString(digest[:8])}
}

func (s *windowsCredentialStore) Name() string     { return "Windows Credential Manager" }
func (s *windowsCredentialStore) Persistent() bool { return true }

func (s *windowsCredentialStore) target(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("model provider id cannot be empty")
	}
	if strings.ContainsRune(id, '\x00') {
		return "", errors.New("model provider id contains an invalid character")
	}
	return s.namespace + "." + id, nil
}

func (s *windowsCredentialStore) Set(ctx context.Context, id, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(value) == "" {
		return errors.New("API key cannot be empty")
	}
	blob := []byte(value)
	if len(blob) > credentialMaxBlobSize {
		return fmt.Errorf("API key is too large for Windows Credential Manager: %d bytes", len(blob))
	}
	defer clear(blob)
	target, err := s.target(id)
	if err != nil {
		return err
	}
	targetPtr, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return fmt.Errorf("encode credential target: %w", err)
	}
	userPtr, err := windows.UTF16PtrFromString(strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("encode credential owner: %w", err)
	}
	credential := windowsCredential{
		Type:               credentialTypeGeneric,
		TargetName:         targetPtr,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credentialPersistLocal,
		UserName:           userPtr,
	}
	result, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&credential)), 0)
	runtime.KeepAlive(blob)
	runtime.KeepAlive(targetPtr)
	runtime.KeepAlive(userPtr)
	if result == 0 {
		return fmt.Errorf("could not store API key in Windows Credential Manager: %w", callErr)
	}
	return nil
}

func (s *windowsCredentialStore) Get(ctx context.Context, id string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	target, err := s.target(id)
	if err != nil {
		return "", err
	}
	targetPtr, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return "", fmt.Errorf("encode credential target: %w", err)
	}
	var credential *windowsCredential
	result, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		credentialTypeGeneric,
		0,
		uintptr(unsafe.Pointer(&credential)),
	)
	runtime.KeepAlive(targetPtr)
	if result == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return "", ErrSecretNotFound
		}
		return "", fmt.Errorf("could not read API key from Windows Credential Manager: %w", callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(credential)))
	if credential == nil || credential.CredentialBlob == nil || credential.CredentialBlobSize == 0 {
		return "", ErrSecretNotFound
	}
	if credential.CredentialBlobSize > credentialMaxBlobSize {
		return "", errors.New("Windows Credential Manager returned an oversized API key")
	}
	value := string(unsafe.Slice(credential.CredentialBlob, int(credential.CredentialBlobSize)))
	if value == "" {
		return "", ErrSecretNotFound
	}
	return value, nil
}

func (s *windowsCredentialStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := s.target(id)
	if err != nil {
		return err
	}
	targetPtr, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return fmt.Errorf("encode credential target: %w", err)
	}
	result, _, callErr := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		credentialTypeGeneric,
		0,
	)
	runtime.KeepAlive(targetPtr)
	if result == 0 && !errors.Is(callErr, windows.ERROR_NOT_FOUND) {
		return fmt.Errorf("could not delete API key from Windows Credential Manager: %w", callErr)
	}
	return nil
}
