//go:build windows

package operations

import "syscall"

// daemonSysProcAttr returns the SysProcAttr for the daemon child process on
// Windows
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x00000008} // CREATE_NEW_PROCESS_GROUP
}
