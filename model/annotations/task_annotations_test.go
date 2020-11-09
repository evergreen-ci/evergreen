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
			Id:             "1",
			TaskId:         "t1",
			TaskExecution:  2,
			APIAnnotation:  &Annotation{Note: &Note{Message: "this is a note"}},
			UserAnnotation: &Annotation{Note: &Note{Message: "this will not be returned"}},
		},
		{
			Id:            "2",
			TaskId:        "t1",
			TaskExecution: 1,
			APIAnnotation: &Annotation{Note: &Note{Message: "another note"}},
		},
		{
			Id:             "3",
			TaskId:         "t1",
			TaskExecution:  0,
			UserAnnotation: &Annotation{Note: &Note{Message: "only a user annotation"}},
		},
		{
			Id:            "4",
			TaskId:        "t2",
			TaskExecution: 0,
			APIAnnotation: &Annotation{Note: &Note{Message: "this is the wrong task"}},
		},
	}
	for _, a := range annotations {
		assert.NoError(t, a.Insert())
	}

	annotations, err := FindAPIAnnotationsByTaskIds([]string{"t1", "t2"})
	assert.NoError(t, err)
	assert.Len(t, annotations, 3)
	for _, a := range annotations {
		assert.Nil(t, a.UserAnnotation)
	}
	assert.Len(t, GetLatestExecutions(annotations), 2)
}
