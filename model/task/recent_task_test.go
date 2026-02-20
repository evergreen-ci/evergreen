package task

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
)

func TestRecentTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	err := db.Clear(Collection)
	assert.NoError(err)

	tasks := []Task{}
	for i := 0; i < 5; i++ {
		tasks = append(tasks, Task{
			Id:            fmt.Sprintf("taskid-%d", i),
			Secret:        fmt.Sprintf("secret-%d", i),
			CreateTime:    time.Now(),
			DispatchTime:  time.Now(),
			ScheduledTime: time.Now(),
			StartTime:     time.Now(),
			FinishTime:    time.Now(),
			Version:       fmt.Sprintf("version-%d", i),
			Project:       fmt.Sprintf("project-%d", i),
			Revision:      fmt.Sprintf("revision-%d", i),
		})
	}

	for _, task := range tasks {
		err = task.Insert(t.Context())
		assert.NoError(err)
	}

	recent, err := GetRecentTasks(ctx, 1*time.Minute)
	assert.NoError(err)
	assert.Len(recent, 5)
}
