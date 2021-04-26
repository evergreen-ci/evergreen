package queue

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/mongodb/amboy"
)

// randomString returns a cryptographically random string.
func randomString(x int) string {
	b := make([]byte, x)
	_, _ = rand.Read(b) // nolint
	return hex.EncodeToString(b)
}

func addJobsSuffix(s string) string {
	return s + ".jobs"
}

func trimJobsSuffix(s string) string {
	return strings.TrimSuffix(s, ".jobs")
}

func addGroupSuffix(s string) string {
	return s + ".group"
}

func isDispatchable(stat amboy.JobStatusInfo, lockTimeout time.Duration) bool {
	if isStaleJob(stat, lockTimeout) {
		return true
	}
	if stat.Completed {
		return false
	}
	if stat.InProgress {
		return false
	}

	return true
}

func isStaleJob(stat amboy.JobStatusInfo, lockTimeout time.Duration) bool {
	return stat.InProgress && time.Since(stat.ModificationTime) > lockTimeout
}
