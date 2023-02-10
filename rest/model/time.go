package model

import (
	"io"
	"strconv"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

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
		return APIDuration(0), errors.Errorf("programmatic error: expected an integer duration (in nanoseconds) but got type %T", v)
	}
}

func ToTimePtr(t time.Time) *time.Time {
	if utility.IsZeroTime(t) {
		return nil
	}
	return &t
}

func FromTimePtr(t *time.Time) (time.Time, error) {
	if t == nil {
		return time.Time{}, nil
	}

	return model.ParseTime(t.Format(evergreen.APITimeFormat))
}
