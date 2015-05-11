package util

import (
	"fmt"
	"runtime"
	"testing"
)

// HandleTestingErr catches errors that we do not want to treat
// as relevant a goconvey statement. HandleTestingErr is used
// to terminate unit tests that fail for reasons that are orthogonal to
// the test (filesystem errors, database errors, etc).
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
