package util

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

var DEFAULT_LOG_PATH = filepath.Join(os.Getenv("mci_home"), "logs", "mongo_log.output")

func HandleTestingErr(err error, t *testing.T, format string, a ...interface{}) {
	if err != nil {
		t.Fatalf("%q: %v", fmt.Sprintf(format, a), err)
	}
}
