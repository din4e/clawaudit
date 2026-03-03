//go:build windows

package sandbox

import (
	"os/exec"
)

// execPlatform sets platform-specific process attributes (Windows version)
// Windows doesn't support setpgid, so this is a no-op
func (s *Sandbox) execPlatform(cmd *exec.Cmd) {
	// Windows doesn't support process group isolation the same way
	// Process isolation is handled through job objects in Windows
	// For now, this is a no-op
}
