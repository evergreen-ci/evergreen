package util

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

func DebugTrace() []byte {
	var buf [32 * 1024]byte
	n := runtime.Stack(buf[:], true)
	return buf[:n]
}

// DumpStackOnSIGQUIT listens for a SIGQUIT signal and writes stack dump to the
// given io.Writer when one is received. Blocks, so spawn it as a goroutine.
func DumpStackOnSIGQUIT(out io.Writer) {
	in := make(chan os.Signal)
	signal.Notify(in, syscall.SIGQUIT)
	for _ = range in {
		stackBytes := DebugTrace()
		stackTraceTimeStamped := fmt.Sprintf("%v: %s", time.Now(), string(stackBytes))
		out.Write([]byte(stackTraceTimeStamped))
	}
}
