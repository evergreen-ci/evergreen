package util

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

var DEFAULT_LOG_PATH = filepath.Join(os.Getenv("mci_home"), "logs", "mongo_log.output")

func HandleTestingErr(err error, t *testing.T, format string, a ...interface{}) {
	if err != nil {
		_, file, line, ok := runtime.Caller(1)
		if ok {
			t.Fatalf("%v:%v: %q: %v", file, line, fmt.Sprintf(format, a), err)
		} else {
			t.Fatalf("%q: %v", fmt.Sprintf(format, a), err)
		}
	}
}
