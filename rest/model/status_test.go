package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
)

func TestAPIRecentTaskStatsListBuildFromService(t *testing.T) {
	assert := assert.New(t)
	service := task.ResultCountList{
		Total:    []task.Stat{{Name: "d1", Count: 5}, {Name: "d2", Count: 3}},
		Inactive: []task.Stat{{Name: "d1", Count: 3}, {Name: "d2", Count: 2}},
		Failed:   []task.Stat{{Name: "d1", Count: 2}, {Name: "d2", Count: 1}},
	}

	apiList := APIRecentTaskStatsList{}
	assert.NoError(apiList.BuildFromService(service))
	assert.Equal(ToAPIString("d1"), apiList.Total[0].Name)
	assert.Equal(5, apiList.Total[0].Count)
	assert.Equal(ToAPIString("d1"), apiList.Inactive[0].Name)
	assert.Equal(3, apiList.Inactive[0].Count)
	assert.Empty(apiList.Succeeded)
}
