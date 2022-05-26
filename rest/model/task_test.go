package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
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
		caTime := timeNow.Add(14*time.Minute + 30*time.Second)
		fTime := timeNow.Add(15 * time.Minute)
		modelPairs := []taskCompare{
			{
				at: APITask{
					Id:                     utility.ToStringPtr("testId"),
					CreateTime:             &cTime,
					DispatchTime:           &dTime,
					ScheduledTime:          &scTime,
					ContainerAllocatedTime: &caTime,
					StartTime:              &sTime,
					FinishTime:             &fTime,
					IngestTime:             &timeNow,
					Version:                utility.ToStringPtr("testVersion"),
					Revision:               utility.ToStringPtr("testRevision"),
					ProjectId:              utility.ToStringPtr("testProject"),
					Priority:               100,
					Execution:              2,
					Activated:              true,
					ActivatedBy:            utility.ToStringPtr("testActivator"),
					BuildId:                utility.ToStringPtr("testBuildId"),
					DistroId:               utility.ToStringPtr("testDistroId"),
					BuildVariant:           utility.ToStringPtr("testBuildVariant"),
					DependsOn: []APIDependency{
						{TaskId: "testDepends1", Status: "*", Unattainable: false},
						{TaskId: "testDepends2", Status: "*", Unattainable: true},
					},
					DisplayName: utility.ToStringPtr("testDisplayName"),
					Logs: LogLinks{
						AllLogLink:    utility.ToStringPtr("url/task_log_raw/testId/2?type=ALL"),
						TaskLogLink:   utility.ToStringPtr("url/task_log_raw/testId/2?type=T"),
						SystemLogLink: utility.ToStringPtr("url/task_log_raw/testId/2?type=S"),
						AgentLogLink:  utility.ToStringPtr("url/task_log_raw/testId/2?type=E"),
					},
				},
				st: task.Task{
					Id:                     "testId",
					Project:                "testProject",
					CreateTime:             cTime,
					DispatchTime:           dTime,
					ScheduledTime:          scTime,
					ContainerAllocatedTime: caTime,
					StartTime:              sTime,
					FinishTime:             fTime,
					IngestTime:             timeNow,
					Version:                "testVersion",
					Revision:               "testRevision",
					Execution:              2,
					Priority:               100,
					Activated:              true,
					ActivatedBy:            "testActivator",
					BuildId:                "testBuildId",
					DistroId:               "testDistroId",
					BuildVariant:           "testBuildVariant",
					DependsOn: []task.Dependency{
						{
							TaskId:       "testDepends1",
							Unattainable: false,
						},
						{
							TaskId:       "testDepends2",
							Unattainable: true,
						},
					},
					DisplayName: "testDisplayName",
					Requester:   evergreen.RepotrackerVersionRequester,
				},
			},
			{
				at: APITask{
					Logs: LogLinks{
						AllLogLink:    utility.ToStringPtr("url/task_log_raw//0?type=ALL"),
						TaskLogLink:   utility.ToStringPtr("url/task_log_raw//0?type=T"),
						SystemLogLink: utility.ToStringPtr("url/task_log_raw//0?type=S"),
						AgentLogLink:  utility.ToStringPtr("url/task_log_raw//0?type=E"),
						EventLogLink:  utility.ToStringPtr("url/event_log/task/"),
					},
					CreateTime:             &time.Time{},
					DispatchTime:           &time.Time{},
					ScheduledTime:          &time.Time{},
					ContainerAllocatedTime: &time.Time{},
					StartTime:              &time.Time{},
					FinishTime:             &time.Time{},
					IngestTime:             &time.Time{},
					DependsOn: []APIDependency{
						{TaskId: "testDepends1", Status: "*", Unattainable: false},
						{TaskId: "testDepends2", Status: "*", Unattainable: true},
					},
				},
				st: task.Task{
					Requester: evergreen.RepotrackerVersionRequester,
					DependsOn: []task.Dependency{
						{
							TaskId:       "testDepends1",
							Unattainable: false,
						},
						{
							TaskId:       "testDepends2",
							Unattainable: true,
						},
					},
				},
			},
		}
		Convey("running BuildFromService(), should produce the equivalent model", func() {
			for _, tc := range modelPairs {
				apiTask := &APITask{}
				err := apiTask.BuildFromService(&tc.st)
				So(err, ShouldBeNil)
				So(true, ShouldEqual, apiTask.Mainline)
				So(apiTask.Blocked, ShouldBeTrue)

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
			}
		})
		Convey("running BuildFromArgs(), should produce an equivalent model", func() {
			for _, tc := range modelPairs {
				apiTask := &APITask{}
				err := apiTask.BuildFromArgs(&tc.st, nil)
				So(err, ShouldBeNil)
				So(true, ShouldEqual, apiTask.Mainline)
				So(apiTask.Blocked, ShouldBeTrue)

				tc.st.Requester = evergreen.PatchVersionRequester
				apiTask = &APITask{}
				err = apiTask.BuildFromArgs(&tc.st, nil)
				So(err, ShouldBeNil)
				So(false, ShouldEqual, apiTask.Mainline)

				tc.st.Requester = evergreen.GithubPRRequester
				apiTask = &APITask{}
				err = apiTask.BuildFromArgs(&tc.st, nil)
				So(err, ShouldBeNil)
				So(false, ShouldEqual, apiTask.Mainline)

				tc.st.Requester = evergreen.TriggerRequester
				apiTask = &APITask{}
				err = apiTask.BuildFromArgs(&tc.st, nil)
				So(err, ShouldBeNil)
				So(false, ShouldEqual, apiTask.Mainline)

				tc.st.Requester = evergreen.AdHocRequester
				apiTask = &APITask{}
				err = apiTask.BuildFromArgs(&tc.st, nil)
				So(err, ShouldBeNil)
				So(false, ShouldEqual, apiTask.Mainline)

				err = apiTask.BuildFromArgs(&tc.st, &APITaskArgs{LogURL: "url"})
				So(err, ShouldBeNil)
				So(utility.FromStringPtr(apiTask.Id), ShouldEqual, utility.FromStringPtr(tc.at.Id))
				So(apiTask.Execution, ShouldEqual, tc.at.Execution)
				So(utility.FromStringPtr(apiTask.Logs.AgentLogLink), ShouldEqual, utility.FromStringPtr(tc.at.Logs.AgentLogLink))
				So(utility.FromStringPtr(apiTask.Logs.SystemLogLink), ShouldEqual, utility.FromStringPtr(tc.at.Logs.SystemLogLink))
				So(utility.FromStringPtr(apiTask.Logs.TaskLogLink), ShouldEqual, utility.FromStringPtr(tc.at.Logs.TaskLogLink))
			}
		})
	})
}
