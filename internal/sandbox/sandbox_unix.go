//go:build !windows

package sandbox

import (
	"os/exec"
	"syscall"
)

// execPlatform sets platform-specific process attributes (Unix version)
func (s *Sandbox) execPlatform(cmd *exec.Cmd) {
	// Set process group for isolation (Unix only)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}
