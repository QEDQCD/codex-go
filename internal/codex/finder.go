package codex

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

var commonPaths = map[string][]string{
	"windows": {
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Codex", "codex.exe"),
		filepath.Join(os.Getenv("APPDATA"), "npm", "codex.cmd"),
	},
	"darwin": {
		"/usr/local/bin/codex",
		"/opt/homebrew/bin/codex",
	},
	"linux": {
		"/usr/local/bin/codex",
		"/usr/bin/codex",
		filepath.Join(os.Getenv("HOME"), ".local/bin/codex"),
		filepath.Join(os.Getenv("HOME"), ".npm-global/bin/codex"),
	},
}

func FindCodexCLI() (string, error) {
	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}
	paths := commonPaths[runtime.GOOS]
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

func ValidateCodexCLI(path string) (string, error) {
	cmd := exec.Command(path, "--version")
	setHideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
