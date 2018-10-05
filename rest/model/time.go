package model

import (
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/util"
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

type APITime time.Time

// Represents duration in milliseconds
type APIDuration uint64

var APIZeroTime APITime = APITime(util.ZeroTime.UTC())

// NewTime creates a new APITime from an existing time.Time. It handles changing
// converting from the times time zone to UTC.
func NewTime(t time.Time) APITime {
	return APITime(t.In(time.UTC))
}

// UnmarshalJSON implements the custom unmarshalling of this type so that it can
// be correctly parsed from an API request.
func (at *APITime) UnmarshalJSON(b []byte) error {
	str := string(b)
	t := time.Time{}
	var err error
	if str != "null" {
		t, err = ParseTime(str)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	(*at) = APITime(t)
	return nil

}

// MarshalJSON implements the custom marshalling of this type so that it can
// be correctly written out in an API response.
func (at APITime) MarshalJSON() ([]byte, error) {
	t := time.Time(at)
	if util.IsZeroTime(t) {
		return []byte("null"), nil
	}
	return []byte(t.Format(APITimeFormat)), nil
}

func (at APITime) String() string {
	return time.Time(at).Format(APITimeFormat)
}

func NewAPIDuration(d time.Duration) APIDuration {
	return APIDuration(d / time.Millisecond)
}

func (i APIDuration) ToDuration() time.Duration {
	return time.Duration(i) * time.Millisecond
}
