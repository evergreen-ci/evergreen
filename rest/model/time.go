package model

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
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

func MarshalAPIDuration(b APIDuration) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		w.Write([]byte(strconv.FormatInt(int64(b), 10)))
	})
}

func UnmarshalAPIDuration(v interface{}) (APIDuration, error) {
	switch v := v.(type) {
	case int:
		return APIDuration(v), nil
	default:
		return APIDuration(0), fmt.Errorf("%T is not an APIDuration", v)
	}
}
