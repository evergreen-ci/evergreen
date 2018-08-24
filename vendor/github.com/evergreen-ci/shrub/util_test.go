package shrub

import (
	"strings"
	"testing"
)

func prepend(s string, l []string) []string {
	if l == nil {
		return nil
	}

	return append([]string{s}, l...)
}

func assert(t *testing.T, cond bool, msgElems ...string) {
	if !cond {
		t.Error(prepend("assert failed:", msgElems))
	}
}

func require(t *testing.T, cond bool, msgElems ...string) {
	if !cond {
		t.Fatal(prepend("failed require:", msgElems))
	}
}

func catch(t *testing.T, msgElems ...string) {
	if r := recover(); r != nil {
		t.Fatalf("panic '%v' %s", r, strings.Join(prepend("in", msgElems), " "))
	}
}

func expect(t *testing.T, msgElems ...string) {
	if r := recover(); r == nil {
		t.Fatalf("panic expected in op '%s' but did not", strings.Join(prepend("in", msgElems), " "))
	}

}
