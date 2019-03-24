// +build darwin linux

package jasper

import "syscall"

func makeCompatible(sig syscall.Signal) syscall.Signal {
	return sig
}
