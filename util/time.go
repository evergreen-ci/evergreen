package util

import "time"

// TimeIsWithinLastDay indicates if the timestamp occurred within the last day.
func TimeIsWithinLastDay(timestamp time.Time) bool {
	now := time.Now()
	oneDayAgo := now.Add(-1 * 24 * time.Hour)
	return timestamp.After(oneDayAgo)
}
