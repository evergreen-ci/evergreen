package units

import (
	"testing"

	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCreatePodJob(t *testing.T) {
	podID := utility.RandomString()
	p := pod.Pod{
		ID:     podID,
		Status: pod.StatusInitializing,
	}
	j, ok := NewCreatePodJob(&mock.Environment{}, &p, utility.RoundPartOfMinute(0).Format(TSFormat)).(*createPodJob)
	require.True(t, ok)

	assert.NotZero(t, j.ID())
	assert.Equal(t, podID, j.PodID)
	assert.Equal(t, pod.StatusInitializing, j.pod.Status)
}
func TestCreatePodJob(t *testing.T) {

}
