package poplar

import (
	"crypto/rand"
	"encoding/hex"
	"reflect"
	"runtime"
	"strings"
	"time"
)

func isMoreThanOneTrue(in []bool) bool {
	count := 0
	for _, v := range in {
		if v {
			count++
		}
		if count > 1 {
			return true
		}
	}

	return false
}

func getName(i interface{}) string {
	n := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
	parts := strings.Split(n, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}

	return n
}

func roundDurationMS(d time.Duration) time.Duration {
	rounded := d.Round(time.Millisecond)
	if rounded == 1<<63-1 {
		return 0
	}
	return rounded
}

func randomString() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
