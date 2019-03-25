// +build darwin linux freebsd

package jasper

import "syscall"

func makeCompatible(sig syscall.Signal) syscall.Signal {
	return sig
}
