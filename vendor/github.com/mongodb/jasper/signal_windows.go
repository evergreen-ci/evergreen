package jasper

import "syscall"

func makeCompatible(sig syscall.Signal) syscall.Signal {
	switch sig {
	case syscall.SIGTERM:
		return syscall.SIGKILL
	case syscall.SIGABRT:
		return syscall.SIGKILL
	default:
		return sig
	}
}
