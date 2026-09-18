//go:build !windows

package monitor

import "os/exec"

func configureCommand(cmd *exec.Cmd) {}
