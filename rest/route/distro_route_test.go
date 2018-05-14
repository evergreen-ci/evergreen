package route

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func init() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func TestClearTaskQueueRoute(t *testing.T) {
	assert := assert.New(t)
	route := clearTaskQueueHandler{}
	distro := "d1"
	tasks := []model.TaskQueueItem{
		{
			Id: "task1",
		},
		{
			Id: "task2",
		},
		{
			Id: "task3",
		},
	}
	queue := model.NewTaskQueue(distro, tasks)
	assert.Len(queue.Queue, 3)
	assert.NoError(queue.Save())

	route.distro = distro
	sc := &data.DBConnector{}
	_, err := route.Execute(context.Background(), sc)
	assert.NoError(err)

	queueFromDb, err := model.LoadTaskQueue(distro)
	assert.NoError(err)
	assert.Len(queueFromDb.Queue, 0)
}
