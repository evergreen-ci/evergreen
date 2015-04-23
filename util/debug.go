package util

import (
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

//dumpStackOnSIGQUIT listens for a SIGQUIT signal and writes stack dump to the
//given io.Writer when one is received. Blocks, so spawn it as a goroutine.
func DumpStackOnSIGQUIT(stackOut io.Writer) {
	in := make(chan os.Signal)
	signal.Notify(in, syscall.SIGQUIT)
	var buf [4096]byte
	for _ = range in {
		n := runtime.Stack(buf[:], true)
		stackOut.Write(buf[:n])
	}
}
