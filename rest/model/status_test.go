package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestAPIRecentTaskStatsListBuildFromService(t *testing.T) {
	assert := assert.New(t)
	service := make(map[string][]task.Stat)
	service["total"] = []task.Stat{{Name: "d1", Count: 5}, {Name: "d2", Count: 3}}
	service[evergreen.TaskInactive] = []task.Stat{{Name: "d1", Count: 3}, {Name: "d2", Count: 2}}
	service[evergreen.TaskFailed] = []task.Stat{{Name: "d1", Count: 2}, {Name: "d2", Count: 1}}

	apiList := APIRecentTaskStatsList{}
	apiList.BuildFromService(service)
	assert.Equal(utility.ToStringPtr("d1"), apiList["total"][0].Name)
	assert.Equal(5, apiList["total"][0].Count)
	assert.Equal(utility.ToStringPtr("d1"), apiList[evergreen.TaskInactive][0].Name)
	assert.Equal(3, apiList[evergreen.TaskInactive][0].Count)
	assert.Empty(apiList[evergreen.TaskSucceeded])
}
