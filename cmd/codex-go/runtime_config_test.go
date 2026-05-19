package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/linfree/codex-go/internal/config"
)

func TestApplyRuntimeConfigOverrides(t *testing.T) {
	origHome := os.Getenv("HOME")
	origProfile := os.Getenv("USERPROFILE")
	origPort := os.Getenv("CODEX_GO_WEB_PORT")
	origRandom := os.Getenv("CODEX_GO_RANDOM_HIGH_PORT")
	origBrowser := os.Getenv("CODEX_GO_DISABLE_BROWSER")
	tmpDir := t.TempDir()
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("USERPROFILE", tmpDir)
	_ = os.Setenv("CODEX_GO_WEB_PORT", "41001")
	_ = os.Setenv("CODEX_GO_RANDOM_HIGH_PORT", "")
	_ = os.Setenv("CODEX_GO_DISABLE_BROWSER", "1")
	defer func() {
		_ = os.Setenv("HOME", origHome)
		_ = os.Setenv("USERPROFILE", origProfile)
		_ = os.Setenv("CODEX_GO_WEB_PORT", origPort)
		_ = os.Setenv("CODEX_GO_RANDOM_HIGH_PORT", origRandom)
		_ = os.Setenv("CODEX_GO_DISABLE_BROWSER", origBrowser)
	}()

	cfg := config.DefaultConfig()
	cfg.AutoOpenBrowser = true
	if err := applyRuntimeConfig(cfg); err != nil {
		t.Fatalf("applyRuntimeConfig: %v", err)
	}
	if cfg.WebPort != 41001 {
		t.Fatalf("expected WebPort 41001, got %d", cfg.WebPort)
	}
	if cfg.AutoOpenBrowser {
		t.Fatalf("expected AutoOpenBrowser false")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".codex-go", "config.json")); err != nil {
		t.Fatalf("expected config.json to be written: %v", err)
	}
}

func TestRandomHighPortRange(t *testing.T) {
	port, err := randomHighPort()
	if err != nil {
		t.Fatalf("randomHighPort: %v", err)
	}
	if port < randomPortMin || port > randomPortMax {
		t.Fatalf("expected port in range, got %d", port)
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", "0"))
	if err != nil {
		t.Fatalf("listen sanity check: %v", err)
	}
	_ = ln.Close()
}
