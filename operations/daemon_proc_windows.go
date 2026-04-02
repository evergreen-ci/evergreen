//go:build windows

package operations

import "syscall"

// daemonSysProcAttr returns the SysProcAttr for the daemon child process on
// Windows. In practice, the debug daemon is only supported on Linux spawn hosts
// and this code path is unreachable; it exists only to satisfy cross-compilation.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x00000008} // CREATE_NEW_PROCESS_GROUP
}
