package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
)

func init() {
	testutil.Setup()
}

func TestAddDisplayTasksToPatchReq(t *testing.T) {
	p := model.Project{
		BuildVariants: []model.BuildVariant{
			{
				Name: "bv",
				DisplayTasks: []patch.DisplayTask{
					{Name: "dt1", ExecTasks: []string{"1", "2"}},
					{Name: "dt2", ExecTasks: []string{"3", "4"}},
				}},
		},
	}
	req := PatchUpdate{
		VariantsTasks: []patch.VariantTasks{
			{Variant: "bv", Tasks: []string{"t1", "dt1", "dt2"}},
		},
	}
	addDisplayTasksToPatchReq(&req, p)
	assert.Len(t, req.VariantsTasks[0].Tasks, 1)
	assert.Equal(t, "t1", req.VariantsTasks[0].Tasks[0])
	assert.Len(t, req.VariantsTasks[0].DisplayTasks, 2)
}
