package annotations

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestGetLatestExecutions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testutil.NewEnvironment(ctx, t)
	assert.NoError(t, db.Clear(Collection))
	annotations := []TaskAnnotation{
		{
			Id:            "1",
			TaskId:        "t1",
			TaskExecution: 2,
			Note:          &Note{Message: "this is a note"},
		},
		{
			Id:            "2",
			TaskId:        "t1",
			TaskExecution: 1,
			Note:          &Note{Message: "another note"},
		},
		{
			Id:            "3",
			TaskId:        "t2",
			TaskExecution: 0,
			Note:          &Note{Message: "this is the wrong task"},
		},
	}
	for _, a := range annotations {
		assert.NoError(t, a.Insert())
	}

	annotations, err := FindAnnotationsByTaskIds([]string{"t1", "t2"})
	assert.NoError(t, err)
	assert.Len(t, annotations, 3)
	assert.Len(t, GetLatestExecutions(annotations), 2)
}
