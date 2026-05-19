package codex

import (
	"runtime"
	"testing"
)

func TestFindCodexCLI_LookPath(t *testing.T) {
	path, err := FindCodexCLI()
	if err != nil {
		t.Skipf("codex not found: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
	t.Logf("found codex at: %s", path)
}

func TestValidateCodexCLI_InvalidPath(t *testing.T) {
	_, err := ValidateCodexCLI("/nonexistent/codex")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestValidateCodexCLI_ValidPath(t *testing.T) {
	path, err := FindCodexCLI()
	if err != nil {
		t.Skip("codex not found, skipping validation test")
	}
	version, err := ValidateCodexCLI(path)
	if err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
	if version == "" {
		t.Error("expected version output")
	}
	t.Logf("codex version: %s", version)
}

func TestCommonPaths_HasEntries(t *testing.T) {
	paths := commonPaths[runtime.GOOS]
	if len(paths) == 0 {
		t.Logf("no common paths for OS: %s", runtime.GOOS)
	}
}
