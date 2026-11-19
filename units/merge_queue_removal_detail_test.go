package units

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeQueueRemovalDetailJobHandleRemovalEvent(t *testing.T) {
	removedAt := time.Now()
	for tName, tCase := range map[string]struct {
		event          *thirdparty.MergeQueueRemovalEvent
		currentAttempt int
		expectRetry    bool
	}{
		"RecentEventShouldNotRetry": {
			event:       &thirdparty.MergeQueueRemovalEvent{Reason: "failed_checks", CreatedAt: removedAt.Add(-time.Second)},
			expectRetry: false,
		},
		"StaleEventWithAttemptsLeftShouldRetry": {
			event:       &thirdparty.MergeQueueRemovalEvent{Reason: "manual", CreatedAt: removedAt.Add(-time.Hour)},
			expectRetry: true,
		},
		"MissingEventWithAttemptsLeftShouldRetry": {
			expectRetry: true,
		},
		"MissingEventOnFinalAttemptShouldNotRetry": {
			currentAttempt: 3,
			expectRetry:    false,
		},
	} {
		t.Run(tName, func(t *testing.T) {
			j, ok := NewMergeQueueRemovalDetailJob("mongodb", "mongo", "refs/heads/gh-readonly-queue/main/pr-515-abc", "abc123", thirdparty.MergeQueueReasonInvalidated, removedAt).(*mergeQueueRemovalDetailJob)
			require.True(t, ok)
			j.UpdateRetryInfo(amboy.JobRetryOptions{CurrentAttempt: &tCase.currentAttempt})

			j.handleRemovalEvent(t.Context(), tCase.event)

			assert.Equal(t, tCase.expectRetry, j.RetryInfo().NeedsRetry)
		})
	}
}
