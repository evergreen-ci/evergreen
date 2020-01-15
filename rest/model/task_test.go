package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
)

type taskCompare struct {
	at APITask
	st task.Task
}

func TestTaskBuildFromService(t *testing.T) {
	Convey("With a list of models to compare", t, func() {
		timeNow := time.Now()
		cTime := timeNow.Add(10 * time.Minute)
		dTime := timeNow.Add(11 * time.Minute)
		sTime := timeNow.Add(13 * time.Minute)
		scTime := timeNow.Add(14 * time.Minute)
		fTime := timeNow.Add(15 * time.Minute)
		modelPairs := []taskCompare{
			{
				at: APITask{
					Id:            ToStringPtr("testId"),
					CreateTime:    cTime,
					DispatchTime:  dTime,
					ScheduledTime: scTime,
					StartTime:     sTime,
					FinishTime:    fTime,
					IngestTime:    timeNow,
					Version:       ToStringPtr("testVersion"),
					Revision:      ToStringPtr("testRevision"),
					ProjectId:     ToStringPtr("testProject"),
					Priority:      100,
					Execution:     2,
					Activated:     true,
					ActivatedBy:   ToStringPtr("testActivator"),
					BuildId:       ToStringPtr("testBuildId"),
					DistroId:      ToStringPtr("testDistroId"),
					BuildVariant:  ToStringPtr("testBuildVariant"),
					DependsOn: []string{
						"testDepends1",
						"testDepends2",
					},
					DisplayName: ToStringPtr("testDisplayName"),
					Logs: logLinks{
						AllLogLink:    ToStringPtr("url/task_log_raw/testId/2?type=ALL"),
						TaskLogLink:   ToStringPtr("url/task_log_raw/testId/2?type=T"),
						SystemLogLink: ToStringPtr("url/task_log_raw/testId/2?type=S"),
						AgentLogLink:  ToStringPtr("url/task_log_raw/testId/2?type=E"),
					},
				},
				st: task.Task{
					Id:            "testId",
					Project:       "testProject",
					CreateTime:    cTime,
					DispatchTime:  dTime,
					ScheduledTime: scTime,
					StartTime:     sTime,
					FinishTime:    fTime,
					IngestTime:    timeNow,
					Version:       "testVersion",
					Revision:      "testRevision",
					Execution:     2,
					Priority:      100,
					Activated:     true,
					ActivatedBy:   "testActivator",
					BuildId:       "testBuildId",
					DistroId:      "testDistroId",
					BuildVariant:  "testBuildVariant",
					DependsOn: []task.Dependency{
						{
							TaskId: "testDepends1",
						},
						{
							TaskId: "testDepends2",
						},
					},
					DisplayName: "testDisplayName",
					Requester:   evergreen.RepotrackerVersionRequester,
				},
			},
			{
				at: APITask{
					Logs: logLinks{
						AllLogLink:    ToStringPtr("url/task_log_raw//0?type=ALL"),
						TaskLogLink:   ToStringPtr("url/task_log_raw//0?type=T"),
						SystemLogLink: ToStringPtr("url/task_log_raw//0?type=S"),
						AgentLogLink:  ToStringPtr("url/task_log_raw//0?type=E"),
					},
					CreateTime:    time.Time{},
					DispatchTime:  time.Time{},
					ScheduledTime: time.Time{},
					StartTime:     time.Time{},
					FinishTime:    time.Time{},
					IngestTime:    time.Time{},
				},
				st: task.Task{
					Requester: evergreen.RepotrackerVersionRequester,
				},
			},
		}
		Convey("running BuildFromService(), should produce the equivalent model", func() {
			for _, tc := range modelPairs {
				apiTask := &APITask{}
				err := apiTask.BuildFromService(&tc.st)
				So(err, ShouldBeNil)
				So(true, ShouldEqual, apiTask.Mainline)

				tc.st.Requester = evergreen.PatchVersionRequester
				apiTask = &APITask{}
				err = apiTask.BuildFromService(&tc.st)
				So(err, ShouldBeNil)
				So(false, ShouldEqual, apiTask.Mainline)

				tc.st.Requester = evergreen.GithubPRRequester
				apiTask = &APITask{}
				err = apiTask.BuildFromService(&tc.st)
				So(err, ShouldBeNil)
				So(false, ShouldEqual, apiTask.Mainline)

				tc.st.Requester = evergreen.TriggerRequester
				apiTask = &APITask{}
				err = apiTask.BuildFromService(&tc.st)
				So(err, ShouldBeNil)
				So(false, ShouldEqual, apiTask.Mainline)

				tc.st.Requester = evergreen.AdHocRequester
				apiTask = &APITask{}
				err = apiTask.BuildFromService(&tc.st)
				So(err, ShouldBeNil)
				So(false, ShouldEqual, apiTask.Mainline)

				tc.st.DependsOn = []task.Dependency{
					{Unattainable: false},
					{Unattainable: true},
				}
				apiTask = &APITask{}
				err = apiTask.BuildFromService(&tc.st)
				So(err, ShouldBeNil)
				So(apiTask.Blocked, ShouldBeTrue)

				err = apiTask.BuildFromService("url")
				So(err, ShouldBeNil)
				So(FromStringPtr(apiTask.Id), ShouldEqual, FromStringPtr(tc.at.Id))
				So(apiTask.Execution, ShouldEqual, tc.at.Execution)
			}
		})
	})
}
