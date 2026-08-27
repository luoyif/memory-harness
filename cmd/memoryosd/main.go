package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/buildinfo"
	"github.com/luoyif/memory-harness/internal/config"
	"github.com/luoyif/memory-harness/internal/doctor"
	"github.com/luoyif/memory-harness/internal/exporter"
	"github.com/luoyif/memory-harness/internal/mcpserver"
	"github.com/luoyif/memory-harness/internal/server"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version", "--version", "-version":
		fmt.Println(buildinfo.Version)
	case "start":
		must(runStart(os.Args[2:]))
	case "doctor":
		must(runDoctor(os.Args[2:]))
	case "rebuild":
		must(runRebuild(os.Args[2:]))
	case "export":
		must(runExport(os.Args[2:]))
	case "restore":
		must(runRestore(os.Args[2:]))
	case "mcp":
		must(runMCP(os.Args[2:]))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "memoryosd <start|doctor|rebuild|export|restore|mcp|version> [flags]")
}
func must(err error) {
	if err != nil {
		log.Printf("error: %v", err)
		os.Exit(1)
	}
}

func common(fs *flag.FlagSet) (*string, *string) {
	home := fs.String("home", "", "MemoryOS data root (default MEMORYOS_HOME or ~/Documents/Knowledge/MemoryOS)")
	addr := fs.String("addr", "", "loopback listen address")
	return home, addr
}
func openFrom(fs *flag.FlagSet, args []string) (*app.App, config.Config, error) {
	home, addr := common(fs)
	if err := fs.Parse(args); err != nil {
		return nil, config.Config{}, err
	}
	cfg, err := config.Resolve(*home, *addr)
	if err != nil {
		return nil, cfg, err
	}
	a, err := app.Open(cfg)
	return a, cfg, err
}

func runStart(args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	a, cfg, err := openFrom(fs, args)
	if err != nil {
		return err
	}
	defer a.Close()
	s := server.New(a)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() { errc <- s.ListenAndServe() }()
	log.Printf("memoryosd %s listening on http://%s (home=%s)", server.Version, cfg.Addr, cfg.Home)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.Shutdown(shutdownCtx)
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	a, _, err := openFrom(fs, args)
	if err != nil {
		return err
	}
	defer a.Close()
	r, err := doctor.Run(context.Background(), a)
	if err != nil {
		return err
	}
	_ = json.NewEncoder(os.Stdout).Encode(r)
	if r.Status != "pass" {
		return errors.New("doctor failed")
	}
	return nil
}
func runRebuild(args []string) error {
	fs := flag.NewFlagSet("rebuild", flag.ContinueOnError)
	a, _, err := openFrom(fs, args)
	if err != nil {
		return err
	}
	defer a.Close()
	n, err := a.Ledger.RebuildSearch(context.Background())
	if err != nil {
		return err
	}
	documents, err := a.Portfolio.RebuildIndex(context.Background())
	if err != nil {
		return err
	}
	fmt.Printf("rebuild PASS evidence=%d documents=%d\n", n, documents)
	return nil
}
func runExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	out := fs.String("output", "", "output .tar.gz path")
	home, addr := common(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("--output is required")
	}
	cfg, err := config.Resolve(*home, *addr)
	if err != nil {
		return err
	}
	a, err := app.Open(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	abs, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if err := exporter.Create(context.Background(), a, abs); err != nil {
		return err
	}
	fmt.Println(abs)
	return nil
}
func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	input := fs.String("input", "", "MemoryOS .tar.gz export")
	home := fs.String("home", "", "empty restore target")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" || *home == "" {
		return errors.New("--input and --home are required")
	}
	cfg, err := config.Resolve(*home, "")
	if err != nil {
		return err
	}
	archive, err := filepath.Abs(*input)
	if err != nil {
		return err
	}
	if err := exporter.Restore(archive, cfg.Home); err != nil {
		return err
	}
	a, err := app.Open(cfg)
	if err != nil {
		return fmt.Errorf("open restored data: %w", err)
	}
	evidence, err := a.Ledger.RebuildSearch(context.Background())
	if err != nil {
		a.Close()
		return fmt.Errorf("rebuild restored Evidence search: %w", err)
	}
	documents, err := a.Portfolio.RebuildIndex(context.Background())
	if closeErr := a.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("rebuild restored unified search: %w", err)
	}
	fmt.Printf("%s\nrestore PASS evidence=%d documents=%d\n", cfg.Home, evidence, documents)
	return nil
}
func runMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "MemoryOS daemon endpoint (default MEMORYOS_ENDPOINT or http://127.0.0.1:19777)")
	tokenEnv := fs.String("agent-token-env", "MEMORYOS_AGENT_TOKEN", "environment variable containing the scoped Agent token")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *endpoint == "" {
		*endpoint = os.Getenv("MEMORYOS_ENDPOINT")
	}
	if *endpoint == "" {
		*endpoint = "http://" + config.DefaultAddr
	}
	backend, err := mcpserver.NewAgentHTTPBackend(*endpoint, os.Getenv(*tokenEnv))
	if err != nil {
		return err
	}
	if err := backend.ValidateAgent(context.Background()); err != nil {
		return err
	}
	return mcpserver.Run(context.Background(), backend)
}
