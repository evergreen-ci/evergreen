package graphql

import (
	"fmt"
	"time"
)

// GetFormattedDuration returns a time.Duration type in "33h 33m 33s" format
func GetFormattedDuration(d time.Duration) *string {
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	formatted := fmt.Sprintf("%02dh %02dm %02ds", h, m, s)
	return &formatted
}
