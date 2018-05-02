package trigger

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestNotificationGeneratorFailureScenarios(t *testing.T) {
	assert := assert.New(t)
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())

	e := &event.EventLogEntry{
		ResourceType: event.ResourceTypeHost,
	}

	gen := notificationGenerator{}

	n, err := gen.generate(e)
	assert.Empty(n)
	assert.EqualError(err, "trigger name is empty")

	gen.triggerName = "test"
	n, err = gen.generate(e)
	assert.Empty(n)
	assert.EqualError(err, "trigger test has no selectors")

	gen.selectors = []event.Selector{
		{
			Type: "id",
			Data: "special",
		},
	}
	n, err = gen.generate(e)
	assert.Empty(n)
	assert.Error(err, "generator has no payloads, and cannot yield any notifications")
}
