package model

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
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
		_, err := w.Write([]byte(strconv.FormatInt(int64(b), 10)))
		grip.Error(err)
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

func ToTimePtr(t time.Time) *time.Time {
	if util.IsZeroTime(t) {
		return nil
	}
	return &t
}

func FromTimePtr(t *time.Time) (time.Time, error) {
	if t == nil {
		return time.Time{}, nil
	}

	return ParseTime(t.Format(APITimeFormat))
}
