package operations

import (
	"testing"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/assert"
)

func TestAdminCommands(t *testing.T) {
	assert := assert.New(t)

	flags := &model.APIServiceFlags{}
	assert.Zero(*flags)

	assert.Error(setServiceFlagValues([]string{"one"}, true, flags))
	assert.Zero(*flags)
	assert.Error(setServiceFlagValues([]string{"one", "evergreen"}, true, flags))
	assert.Zero(*flags)
	assert.Error(setServiceFlagValues([]string{"one", "runner"}, true, flags))
	assert.Zero(*flags)
	assert.Error(setServiceFlagValues([]string{"one", "all"}, true, flags))
	assert.Zero(*flags)

	// if we set things that were false to false, then it's still zero'd
	assert.NoError(setServiceFlagValues([]string{"github", "scheduler", "commits"}, false, flags))
	assert.Zero(*flags)

	assert.NoError(setServiceFlagValues([]string{"hostinit", "monitor", "agents", "tasks"}, true, flags))
	assert.NotZero(*flags)
	assert.True(flags.TaskrunnerDisabled)
	assert.True(flags.MonitorDisabled)
	assert.True(flags.HostinitDisabled)
	assert.True(flags.TaskDispatchDisabled)

	assert.NoError(setServiceFlagValues([]string{"hostinit", "monitor", "agents", "tasks"}, false, flags))
	assert.Zero(*flags)
}
