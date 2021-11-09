package race

import (
	"runtime"
	"testing"
)

var evilGlobal = 0

func TestRaceFail1(t *testing.T) {
	t.Parallel()
	for i := 0; i < 100; i += 1 {
		temp := evilGlobal
		evilGlobal = temp + 1
		runtime.Gosched()
	}

	if evilGlobal != 100 {
		t.Error("expected evilGlobal to be 100")
	}
}

func TestRaceFail2(t *testing.T) {
	t.Parallel()
	for i := 0; i < 100; i += 1 {
		temp := evilGlobal
		evilGlobal = temp + 1
		runtime.Gosched()
	}

	if evilGlobal != 100 {
		t.Errorf("expected evilGlobal to be 100, was %d", evilGlobal)
	}
}
