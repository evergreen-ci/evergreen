package model

import (
	"testing"
	"time"

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
					Id:            ToApiString("testId"),
					CreateTime:    NewTime(cTime),
					DispatchTime:  NewTime(dTime),
					ScheduledTime: NewTime(scTime),
					StartTime:     NewTime(sTime),
					FinishTime:    NewTime(fTime),
					Version:       ToApiString("testVersion"),
					Branch:        ToApiString("testProject"),
					Revision:      ToApiString("testRevision"),
					Priority:      100,
					Execution:     2,
					Activated:     true,
					ActivatedBy:   ToApiString("testActivator"),
					BuildId:       ToApiString("testBuildId"),
					DistroId:      ToApiString("testDistroId"),
					BuildVariant:  ToApiString("testBuildVariant"),
					DependsOn: []string{
						"testDepends1",
						"testDepends2",
					},
					DisplayName: ToApiString("testDisplayName"),
					Logs: logLinks{
						AllLogLink:    ToApiString("url/task_log_raw/testId/2?type=ALL"),
						TaskLogLink:   ToApiString("url/task_log_raw/testId/2?type=T"),
						SystemLogLink: ToApiString("url/task_log_raw/testId/2?type=S"),
						AgentLogLink:  ToApiString("url/task_log_raw/testId/2?type=E"),
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
				},
			},
			{
				at: APITask{
					Logs: logLinks{
						AllLogLink:    ToApiString("url/task_log_raw//0?type=ALL"),
						TaskLogLink:   ToApiString("url/task_log_raw//0?type=T"),
						SystemLogLink: ToApiString("url/task_log_raw//0?type=S"),
						AgentLogLink:  ToApiString("url/task_log_raw//0?type=E"),
					},
					CreateTime:    NewTime(time.Time{}),
					DispatchTime:  NewTime(time.Time{}),
					ScheduledTime: NewTime(time.Time{}),
					StartTime:     NewTime(time.Time{}),
					FinishTime:    NewTime(time.Time{}),
				},
				st: task.Task{},
			},
		}
		Convey("running BuildFromService(), should produce the equivalent model", func() {
			for _, tc := range modelPairs {
				apiTask := &APITask{}
				err := apiTask.BuildFromService(&tc.st)
				So(err, ShouldBeNil)
				err = apiTask.BuildFromService("url")
				So(err, ShouldBeNil)
				So(FromApiString(apiTask.Id), ShouldEqual, FromApiString(tc.at.Id))
				So(apiTask.Execution, ShouldEqual, tc.at.Execution)
			}
		})
	})
}
