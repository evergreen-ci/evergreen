package units

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/stretchr/testify/assert"
)

func TestPushTriggerVersionsJobShouldShareScopeWithDownstreamRepotracker(t *testing.T) {
	j := NewPushTriggerVersionsJob("msg", "upstream", "downstream", []model.Revision{{Revision: "sha"}}, time.Now())

	assert.Equal(t, "push-trigger-versions:msg:upstream:downstream", j.ID())
	assert.Equal(t, NewRepotrackerJob("other-msg", "downstream").Scopes(), j.Scopes())
	assert.NotEqual(t, NewRepotrackerJob("msg", "upstream").Scopes(), j.Scopes())
}
