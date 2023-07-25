package agent

import (
	"testing"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/stretchr/testify/assert"
)

func TestGetOomTrackerInfo(t *testing.T) {
	tc := taskContext{oomTracker: jasper.NewOOMTracker()}
	info := tc.getOomTrackerInfo()
	assert.Nil(t, info)

	tc.oomTracker = &mock.OOMTracker{Lines: []string{"line1", "line2", "line3"}, PIDs: []int{1, 2, 3}}
	info = tc.getOomTrackerInfo()
	assert.NotNil(t, info)
	assert.True(t, info.Detected)
	assert.Equal(t, []int{1, 2, 3}, info.Pids)
}
