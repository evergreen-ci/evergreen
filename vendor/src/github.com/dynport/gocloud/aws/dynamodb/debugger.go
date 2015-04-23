package dynamodb

import (
	"io"
	"log"
	"os"
)

var debugger = log.New(debugWriter(), "DEBUG:", log.Llongfile)

type NullWriter struct {
}

func (w *NullWriter) Write([]byte) (int, error) {
	return 0, nil
}

func debugWriter() io.Writer {
	if os.Getenv("DEBUG") == "true" {
		return os.Stderr
	}
	return &NullWriter{}
}
