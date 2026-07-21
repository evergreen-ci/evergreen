package evergreen

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

var adminDurationDayOrWeek = regexp.MustCompile(`(?:\d+(?:\.\d*)?|\.\d+)[dw]`)

// ResolveAdminDuration returns the configured duration, falling back to a
// legacy integer value when the duration is zero.
// TODO (DEVPROD-37602): Remove this after the admin duration migration is complete.
func ResolveAdminDuration(value time.Duration, legacyValue int, legacyUnit time.Duration) time.Duration {
	if value != 0 || legacyValue == 0 {
		return value
	}
	return time.Duration(legacyValue) * legacyUnit
}

// ParseAdminDuration parses a duration used by admin settings. In addition to
// units supported by time.ParseDuration, it supports days and weeks.
func ParseAdminDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "0" {
		return 0, nil
	}

	var conversionErr error
	normalized := adminDurationDayOrWeek.ReplaceAllStringFunc(value, func(component string) string {
		unit := component[len(component)-1]
		number, err := strconv.ParseFloat(component[:len(component)-1], 64)
		if err != nil {
			conversionErr = err
			return component
		}
		hours := number * 24
		if unit == 'w' {
			hours *= 7
		}
		return strconv.FormatFloat(hours, 'f', -1, 64) + "h"
	})
	if conversionErr != nil {
		return 0, errors.Wrap(conversionErr, "parsing duration value")
	}
	duration, err := time.ParseDuration(normalized)
	return duration, errors.Wrapf(err, "parsing duration '%s'", value)
}

// ParseAdminDurationWithLegacy parses an admin duration when present, falling
// back to the first non-nil legacy integer value.
// TODO (DEVPROD-37602): Remove this after the admin duration migration is complete.
func ParseAdminDurationWithLegacy(value *string, legacyUnit time.Duration, legacyValues ...*int) (time.Duration, error) {
	if value != nil {
		return ParseAdminDuration(*value)
	}
	for _, legacyValue := range legacyValues {
		if legacyValue != nil {
			return time.Duration(*legacyValue) * legacyUnit, nil
		}
	}
	return 0, nil
}

// FormatAdminDuration formats a duration for admin settings using days for
// durations of at least 24 hours.
func FormatAdminDuration(duration time.Duration) string {
	if duration == 0 {
		return "0s"
	}

	sign := ""
	if duration < 0 {
		sign = "-"
		duration = -duration
	}

	const day = 24 * time.Hour
	days := duration / day
	remainder := duration % day
	if days == 0 {
		return sign + formatSubdayAdminDuration(duration)
	}
	if remainder == 0 {
		return sign + strconv.FormatInt(int64(days), 10) + "d"
	}
	return sign + strconv.FormatInt(int64(days), 10) + "d" + formatSubdayAdminDuration(remainder)
}

// FormatOptionalAdminDuration formats a nonzero admin duration.
func FormatOptionalAdminDuration(duration time.Duration) *string {
	if duration == 0 {
		return nil
	}
	formatted := FormatAdminDuration(duration)
	return &formatted
}

func formatSubdayAdminDuration(duration time.Duration) string {
	if duration%time.Hour == 0 {
		return strconv.FormatInt(int64(duration/time.Hour), 10) + "h"
	}
	if duration%time.Minute == 0 {
		hours := duration / time.Hour
		minutes := duration % time.Hour / time.Minute
		if hours == 0 {
			return strconv.FormatInt(int64(minutes), 10) + "m"
		}
		return strconv.FormatInt(int64(hours), 10) + "h" + strconv.FormatInt(int64(minutes), 10) + "m"
	}
	return duration.String()
}
