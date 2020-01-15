package model

import (
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	// This string defines ISO-8601 UTC with 3 fractional seconds behind a dot
	// specified by the API spec document.
	APITimeFormat = "\"2006-01-02T15:04:05.000Z\""
)

func ParseTime(tval string) (time.Time, error) {
	if !strings.HasPrefix(tval, "\"") {
		tval = "\"" + tval + "\""
	}

	t, err := time.ParseInLocation(APITimeFormat, tval, time.UTC)
	return t, errors.WithStack(err)
}

// Represents duration in milliseconds
type APIDuration uint64

func NewAPIDuration(d time.Duration) APIDuration {
	return APIDuration(d / time.Millisecond)
}

func (i APIDuration) ToDuration() time.Duration {
	return time.Duration(i) * time.Millisecond
}
