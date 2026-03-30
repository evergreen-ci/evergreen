package util

import "time"

// TimeIsWithinLastDay indicates if the timestamp occurred within the last day.
func TimeIsWithinLastDay(timestamp time.Time) bool {
	now := time.Now()
	oneDayAgo := now.Add(-1 * 24 * time.Hour)
	return timestamp.After(oneDayAgo)
}

// NextIntervalBoundary returns the smallest time t such that t >= now and
// t = epoch+k*interval for some integer k >= 0. All times use second resolution
// (nanoseconds are truncated). interval must be at least one second; if not,
// now is returned.
func NextIntervalBoundary(now time.Time, interval time.Duration, epoch time.Time) time.Time {
	intervalSecs := int64(interval / time.Second)
	if intervalSecs <= 0 {
		return now
	}
	epochUnix := epoch.Unix()
	nowUnix := now.Unix()
	elapsed := nowUnix - epochUnix
	if elapsed < 0 {
		return time.Unix(epochUnix, 0).UTC()
	}
	nextUnix := epochUnix + ((elapsed+intervalSecs-1)/intervalSecs)*intervalSecs
	return time.Unix(nextUnix, 0).UTC()
}
