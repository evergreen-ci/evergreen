package evergreen

import (
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// ParseAdminDuration parses a duration used by admin settings. In addition to
// units supported by time.ParseDuration, it supports days and weeks.
func ParseAdminDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "0" {
		return 0, nil
	}
	if value == "" {
		return 0, errors.New("duration must not be empty")
	}

	sign := time.Duration(1)
	if value[0] == '-' || value[0] == '+' {
		if value[0] == '-' {
			sign = -1
		}
		value = value[1:]
	}
	if value == "" {
		return 0, errors.New("duration must include a value")
	}

	adminDurationUnits := [...]string{"ns", "us", "µs", "ms", "s", "m", "h", "d", "w"}
	var total time.Duration
	for len(value) > 0 {
		numberEnd := 0
		hasDecimal := false
		for numberEnd < len(value) {
			switch value[numberEnd] {
			case '.':
				if hasDecimal {
					return 0, errors.Errorf("invalid duration '%s'", value)
				}
				hasDecimal = true
				numberEnd++
			case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
				numberEnd++
			default:
				goto numberComplete
			}
		}

	numberComplete:
		if numberEnd == 0 {
			return 0, errors.Errorf("invalid duration '%s'", value)
		}

		unit := ""
		for _, candidate := range adminDurationUnits {
			if strings.HasPrefix(value[numberEnd:], candidate) {
				unit = candidate
				break
			}
		}
		if unit == "" {
			return 0, errors.Errorf("duration component '%s' has an invalid unit", value)
		}

		component := value[:numberEnd+len(unit)]
		var duration time.Duration
		var err error
		switch unit {
		case "d", "w":
			number, parseErr := strconv.ParseFloat(value[:numberEnd], 64)
			if parseErr != nil {
				return 0, errors.Wrap(parseErr, "parsing duration value")
			}
			hours := number * 24
			if unit == "w" {
				hours *= 7
			}
			duration, err = time.ParseDuration(strconv.FormatFloat(hours, 'f', -1, 64) + "h")
		default:
			duration, err = time.ParseDuration(component)
		}
		if err != nil {
			return 0, errors.Wrapf(err, "parsing duration component '%s'", component)
		}
		if duration > 0 && total > time.Duration(1<<63-1)-duration {
			return 0, errors.New("duration is out of range")
		}
		total += duration
		value = value[len(component):]
	}

	return sign * total, nil
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
