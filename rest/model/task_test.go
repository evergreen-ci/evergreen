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
					Id:            ToAPIString("testId"),
					CreateTime:    NewTime(cTime),
					DispatchTime:  NewTime(dTime),
					ScheduledTime: NewTime(scTime),
					StartTime:     NewTime(sTime),
					FinishTime:    NewTime(fTime),
					IngestTime:    NewTime(timeNow),
					Version:       ToAPIString("testVersion"),
					Revision:      ToAPIString("testRevision"),
					ProjectId:     ToAPIString("testProject"),
					Priority:      100,
					Execution:     2,
					Activated:     true,
					ActivatedBy:   ToAPIString("testActivator"),
					BuildId:       ToAPIString("testBuildId"),
					DistroId:      ToAPIString("testDistroId"),
					BuildVariant:  ToAPIString("testBuildVariant"),
					DependsOn: []string{
						"testDepends1",
						"testDepends2",
					},
					DisplayName: ToAPIString("testDisplayName"),
					Logs: logLinks{
						AllLogLink:    ToAPIString("url/task_log_raw/testId/2?type=ALL"),
						TaskLogLink:   ToAPIString("url/task_log_raw/testId/2?type=T"),
						SystemLogLink: ToAPIString("url/task_log_raw/testId/2?type=S"),
						AgentLogLink:  ToAPIString("url/task_log_raw/testId/2?type=E"),
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
						AllLogLink:    ToAPIString("url/task_log_raw//0?type=ALL"),
						TaskLogLink:   ToAPIString("url/task_log_raw//0?type=T"),
						SystemLogLink: ToAPIString("url/task_log_raw//0?type=S"),
						AgentLogLink:  ToAPIString("url/task_log_raw//0?type=E"),
					},
					CreateTime:    NewTime(time.Time{}),
					DispatchTime:  NewTime(time.Time{}),
					ScheduledTime: NewTime(time.Time{}),
					StartTime:     NewTime(time.Time{}),
					FinishTime:    NewTime(time.Time{}),
					IngestTime:    NewTime(time.Time{}),
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

				err = apiTask.BuildFromService("url")
				So(err, ShouldBeNil)
				So(FromAPIString(apiTask.Id), ShouldEqual, FromAPIString(tc.at.Id))
				So(apiTask.Execution, ShouldEqual, tc.at.Execution)
			}
		})
	})
}
