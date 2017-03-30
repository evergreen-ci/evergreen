package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	. "github.com/smartystreets/goconvey/convey"
)

func TestResourceInfoEndPoints(t *testing.T) {
	testConfig := testutil.TestConfig()
	testApiServer, err := CreateTestServer(testConfig, nil, plugin.APIPlugins, true)
	testutil.HandleTestingErr(err, t, "failed to create new API server")
	defer testApiServer.Close()

	err = db.ClearCollections(event.AllLogCollection, event.TaskLogCollection)
	testutil.HandleTestingErr(err, t, "problem clearing event collection")
	err = db.ClearCollections(task.Collection)
	testutil.HandleTestingErr(err, t, "problem clearing task collection")

	url := fmt.Sprintf("%s/api/2/task/", testApiServer.URL)
	const taskId = "the_task_id"

	_, err = insertTaskForTesting(taskId, "version", "project", task.TestResult{})
	testutil.HandleTestingErr(err, t, "problem creating task")

	Convey("For the system info endpoint", t, func() {
		data := message.CollectSystemInfo().(*message.SystemInfo)
		data.Base = message.Base{}
		data.Errors = []string{}
		Convey("the system info endpoint should return 200", func() {
			payload, err := json.Marshal(data)
			So(err, ShouldBeNil)

			request, err := http.NewRequest("POST", url+taskId+"/system_info", bytes.NewBuffer(payload))
			So(err, ShouldBeNil)
			resp, err := http.DefaultClient.Do(request)
			testutil.HandleTestingErr(err, t, "problem making request")
			So(resp.StatusCode, ShouldEqual, 200)
		})

		Convey("the system data should persist in the database", func() {
			events, err := event.Find(event.TaskLogCollection, event.TaskSystemInfoEvents(taskId, 0))
			testutil.HandleTestingErr(err, t, "problem finding task event")
			So(len(events), ShouldEqual, 1)
			e := events[0]
			So(e.ResourceId, ShouldEqual, taskId)
			taskData, ok := e.Data.Data.(*event.TaskSystemResourceData)
			So(ok, ShouldBeTrue)
			grip.Info(taskData.SystemInfo)
		})
	})

	Convey("For the process info endpoint", t, func() {
		data := message.CollectProcessInfoSelfWithChildren()
		Convey("the process info endpoint should return 200", func() {
			payload, err := json.Marshal(data)
			So(err, ShouldBeNil)

			request, err := http.NewRequest("POST", url+taskId+"/process_info", bytes.NewBuffer(payload))
			resp, err := http.DefaultClient.Do(request)
			testutil.HandleTestingErr(err, t, "problem making request")
			So(resp.StatusCode, ShouldEqual, 200)
		})

		Convey("the process data should persist in the database", func() {
			events, err := event.Find(event.TaskLogCollection, event.TaskProcessInfoEvents(taskId, 0))
			testutil.HandleTestingErr(err, t, "problem finding task event")
			So(len(events), ShouldEqual, 1)
			e := events[0]
			So(e.ResourceId, ShouldEqual, taskId)
			taskData, ok := e.Data.Data.(*event.TaskProcessResourceData)
			So(ok, ShouldBeTrue)
			grip.Info(taskData.Processes)
		})
	})
}
