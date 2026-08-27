package config

import (
	"errors"
	"net"
	"os"
	"path/filepath"
)

const DefaultAddr = "127.0.0.1:19777"

type Config struct {
	Home string
	Addr string
}

func Resolve(home, addr string) (Config, error) {
	if home == "" {
		home = os.Getenv("MEMORYOS_HOME")
	}
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return Config{}, err
		}
		home = filepath.Join(userHome, "Documents", "Knowledge", "MemoryOS")
	}
	if addr == "" {
		addr = os.Getenv("MEMORYOS_ADDR")
	}
	if addr == "" {
		addr = DefaultAddr
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return Config{}, errors.New("addr must be host:port")
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return Config{}, errors.New("MemoryOS M0 only permits loopback listen addresses")
		}
	}
	abs, err := filepath.Abs(home)
	if err != nil {
		return Config{}, err
	}
	if abs == string(filepath.Separator) {
		return Config{}, errors.New("refusing filesystem root as MemoryOS home")
	}
	return Config{Home: abs, Addr: addr}, nil
}

func (c Config) LedgerDir() string  { return filepath.Join(c.Home, "ledger") }
func (c Config) StateDir() string   { return filepath.Join(c.Home, "state") }
func (c Config) CacheDir() string   { return filepath.Join(c.Home, "cache") }
func (c Config) MemoryDir() string  { return filepath.Join(c.Home, "memory") }
func (c Config) SourcesDir() string { return filepath.Join(c.Home, "sources") }
func (c Config) ControlDB() string  { return filepath.Join(c.StateDir(), "control.sqlite") }
func (c Config) SearchDB() string   { return filepath.Join(c.CacheDir(), "search.sqlite") }

func (c Config) EnsureDirs() error {
	for _, p := range []string{c.LedgerDir(), c.StateDir(), c.CacheDir(), c.MemoryDir(), c.SourcesDir()} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			return err
		}
	}
	return nil
}
