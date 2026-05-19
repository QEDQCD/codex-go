//go:build !windows

package codex

import "os/exec"

func setHideWindow(cmd *exec.Cmd) {}
