//go:build !windows

package operations

import "syscall"

// daemonSysProcAttr returns the SysProcAttr used to detach the daemon child
// process from the parent's session on Unix systems.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
