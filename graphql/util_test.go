package graphql

import (
	"context"
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

func TestGetVariantsAndTasksFromProject(t *testing.T) {
	ctx := context.Background()
	patchedConfig := `
buildvariants:
  - name: bv1
    display_name: bv1_display
    run_on:
      - ubuntu1604-test
    tasks:
      - name: task1
        disable: true
      - name: task2
      - name: task3
        depends_on:
          - name: task1
            status: success
tasks:
  - name: task1
  - name: task2
    disable: true
  - name: task3
`
	variantsAndTasks, err := GetVariantsAndTasksFromProject(ctx, patchedConfig, "")
	assert.NoError(t, err)
	assert.Len(t, variantsAndTasks.Variants["bv1"].Tasks, 1)
}
