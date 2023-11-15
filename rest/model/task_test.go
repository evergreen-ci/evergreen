package model

import (
	"context"
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
					Id:                          utility.ToStringPtr("testId"),
					CreateTime:                  &cTime,
					DispatchTime:                &dTime,
					ScheduledTime:               &scTime,
					ContainerAllocatedTime:      &caTime,
					StartTime:                   &sTime,
					FinishTime:                  &fTime,
					IngestTime:                  &timeNow,
					Version:                     utility.ToStringPtr("testVersion"),
					Revision:                    utility.ToStringPtr("testRevision"),
					ProjectId:                   utility.ToStringPtr("testProject"),
					Priority:                    100,
					Execution:                   2,
					Activated:                   true,
					ActivatedBy:                 utility.ToStringPtr("testActivator"),
					ContainerAllocated:          true,
					ContainerAllocationAttempts: 1,
					BuildId:                     utility.ToStringPtr("testBuildId"),
					DistroId:                    utility.ToStringPtr("testDistroId"),
					HostId:                      utility.ToStringPtr("host"),
					PodID:                       utility.ToStringPtr("pod"),
					Container:                   utility.ToStringPtr("container"),
					ContainerOpts: APIContainerOptions{
						CPU:        2048,
						MemoryMB:   4096,
						WorkingDir: utility.ToStringPtr("/working/dir"),
						Image:      utility.ToStringPtr("image"),
						OS:         utility.ToStringPtr(string(evergreen.LinuxOS)),
						Arch:       utility.ToStringPtr(string(evergreen.ArchAMD64)),
					},
					BuildVariant: utility.ToStringPtr("testBuildVariant"),
					DependsOn: []APIDependency{
						{TaskId: "testDepends1", Status: "*"},
						{TaskId: "testDepends2", Status: "*"},
					},
					DisplayName: utility.ToStringPtr("testDisplayName"),
					Logs: LogLinks{
						AllLogLink:    utility.ToStringPtr("url/task_log_raw/testId/2?type=ALL"),
						TaskLogLink:   utility.ToStringPtr("url/task_log_raw/testId/2?type=T"),
						SystemLogLink: utility.ToStringPtr("url/task_log_raw/testId/2?type=S"),
						AgentLogLink:  utility.ToStringPtr("url/task_log_raw/testId/2?type=E"),
					},
					ParsleyLogs: LogLinks{
						AllLogLink:    utility.ToStringPtr("parsley/evergreen/testId/2/all"),
						TaskLogLink:   utility.ToStringPtr("parsley/evergreen/testId/2/task"),
						SystemLogLink: utility.ToStringPtr("parsley/evergreen/testId/2/system"),
						AgentLogLink:  utility.ToStringPtr("parsley/evergreen/testId/2/agent"),
					},
					StepbackInfo: &APIStepbackInfo{
						LastFailingTaskId: "last_failing",
						LastPassingTaskId: "last_passing",
						NextTaskId:        "next",
					},
				},
				st: task.Task{
					Id:                          "testId",
					Project:                     "testProject",
					CreateTime:                  cTime,
					DispatchTime:                dTime,
					ScheduledTime:               scTime,
					ContainerAllocatedTime:      caTime,
					StartTime:                   sTime,
					FinishTime:                  fTime,
					IngestTime:                  timeNow,
					Version:                     "testVersion",
					Revision:                    "testRevision",
					Execution:                   2,
					Priority:                    100,
					Activated:                   true,
					ActivatedBy:                 "testActivator",
					ContainerAllocated:          true,
					ContainerAllocationAttempts: 1,
					BuildId:                     "testBuildId",
					DistroId:                    "testDistroId",
					Container:                   "container",
					ContainerOpts: task.ContainerOptions{
						CPU:        2048,
						MemoryMB:   4096,
						WorkingDir: "/working/dir",
						Image:      "image",
						OS:         evergreen.LinuxOS,
						Arch:       evergreen.ArchAMD64,
					},
					HostId:       "host",
					PodID:        "pod",
					BuildVariant: "testBuildVariant",
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
					StepbackInfo: &task.StepbackInfo{
						LastFailingStepbackTaskId: "last_failing",
						LastPassingStepbackTaskId: "last_passing",
						NextStepbackTaskId:        "next",
					},
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
					ParsleyLogs: LogLinks{
						AllLogLink:    utility.ToStringPtr("parsley/evergreen//0/all"),
						TaskLogLink:   utility.ToStringPtr("parsley/evergreen//0/task"),
						SystemLogLink: utility.ToStringPtr("parsley/evergreen//0/system"),
						AgentLogLink:  utility.ToStringPtr("parsley/evergreen//0/agent"),
					},
					CreateTime:             &time.Time{},
					DispatchTime:           &time.Time{},
					ScheduledTime:          &time.Time{},
					ContainerAllocatedTime: &time.Time{},
					StartTime:              &time.Time{},
					FinishTime:             &time.Time{},
					IngestTime:             &time.Time{},
				},
				st: task.Task{
					Requester: evergreen.RepotrackerVersionRequester,
				},
			},
			{
				at: APITask{
					Id: utility.ToStringPtr("old_task_id"),
					Logs: LogLinks{
						AllLogLink:    utility.ToStringPtr("url/task_log_raw/old_task_id/0?type=ALL"),
						TaskLogLink:   utility.ToStringPtr("url/task_log_raw/old_task_id/0?type=T"),
						SystemLogLink: utility.ToStringPtr("url/task_log_raw/old_task_id/0?type=S"),
						AgentLogLink:  utility.ToStringPtr("url/task_log_raw/old_task_id/0?type=E"),
						EventLogLink:  utility.ToStringPtr("url/event_log/task/old_task_id"),
					},
					ParsleyLogs: LogLinks{
						AllLogLink:    utility.ToStringPtr("parsley/evergreen/old_task_id/0/all"),
						TaskLogLink:   utility.ToStringPtr("parsley/evergreen/old_task_id/0/task"),
						SystemLogLink: utility.ToStringPtr("parsley/evergreen/old_task_id/0/system"),
						AgentLogLink:  utility.ToStringPtr("parsley/evergreen/old_task_id/0/agent"),
					},
					CreateTime:             &time.Time{},
					DispatchTime:           &time.Time{},
					ScheduledTime:          &time.Time{},
					ContainerAllocatedTime: &time.Time{},
					StartTime:              &time.Time{},
					FinishTime:             &time.Time{},
					IngestTime:             &time.Time{},
				},
				st: task.Task{
					Id:        "task_id",
					OldTaskId: "old_task_id",
					Requester: evergreen.RepotrackerVersionRequester,
				},
			},
		}
		Convey("running BuildFromService(), should populate mainline and blocked dependencies", func() {
			for _, tc := range modelPairs {
				apiTask := &APITask{}
				err := apiTask.BuildFromService(ctx, &tc.st, nil)
				So(err, ShouldBeNil)
				So(true, ShouldEqual, apiTask.Mainline)

				tc.st.Requester = evergreen.PatchVersionRequester
				apiTask = &APITask{}
				err = apiTask.BuildFromService(ctx, &tc.st, nil)
				So(err, ShouldBeNil)
				So(false, ShouldEqual, apiTask.Mainline)

				tc.st.Requester = evergreen.GithubPRRequester
				apiTask = &APITask{}
				err = apiTask.BuildFromService(ctx, &tc.st, nil)
				So(err, ShouldBeNil)
				So(false, ShouldEqual, apiTask.Mainline)

				tc.st.Requester = evergreen.TriggerRequester
				apiTask = &APITask{}
				err = apiTask.BuildFromService(ctx, &tc.st, nil)
				So(err, ShouldBeNil)
				So(false, ShouldEqual, apiTask.Mainline)

				tc.st.Requester = evergreen.AdHocRequester
				apiTask = &APITask{}
				err = apiTask.BuildFromService(ctx, &tc.st, nil)
				So(err, ShouldBeNil)
				So(false, ShouldEqual, apiTask.Mainline)

				tc.st.DependsOn = []task.Dependency{
					{Unattainable: false},
					{Unattainable: true},
				}
				apiTask = &APITask{}
				err = apiTask.BuildFromService(ctx, &tc.st, nil)
				So(err, ShouldBeNil)
				So(apiTask.Blocked, ShouldBeTrue)
			}
		})
		Convey("running BuildFromService(), should produce an equivalent model", func() {
			for _, tc := range modelPairs {
				apiTask := &APITask{}
				err := apiTask.BuildFromService(ctx, &tc.st, &APITaskArgs{LogURL: "url", ParsleyLogURL: "parsley"})
				So(err, ShouldBeNil)
				So(utility.FromStringPtr(apiTask.Id), ShouldEqual, utility.FromStringPtr(tc.at.Id))
				So(apiTask.Execution, ShouldEqual, tc.at.Execution)

				So(utility.FromStringPtr(apiTask.Logs.AgentLogLink), ShouldEqual, utility.FromStringPtr(tc.at.Logs.AgentLogLink))
				So(utility.FromStringPtr(apiTask.Logs.SystemLogLink), ShouldEqual, utility.FromStringPtr(tc.at.Logs.SystemLogLink))
				So(utility.FromStringPtr(apiTask.Logs.TaskLogLink), ShouldEqual, utility.FromStringPtr(tc.at.Logs.TaskLogLink))
				So(utility.FromStringPtr(apiTask.Logs.AllLogLink), ShouldEqual, utility.FromStringPtr(tc.at.Logs.AllLogLink))
				So(utility.FromStringPtr(apiTask.ParsleyLogs.TaskLogLink), ShouldEqual, utility.FromStringPtr(tc.at.ParsleyLogs.TaskLogLink))
				So(utility.FromStringPtr(apiTask.ParsleyLogs.SystemLogLink), ShouldEqual, utility.FromStringPtr(tc.at.ParsleyLogs.SystemLogLink))
				So(utility.FromStringPtr(apiTask.ParsleyLogs.AgentLogLink), ShouldEqual, utility.FromStringPtr(tc.at.ParsleyLogs.AgentLogLink))
				So(utility.FromStringPtr(apiTask.ParsleyLogs.AllLogLink), ShouldEqual, utility.FromStringPtr(tc.at.ParsleyLogs.AllLogLink))

				So(utility.FromStringPtr(apiTask.HostId), ShouldEqual, utility.FromStringPtr(tc.at.HostId))
				So(utility.FromStringPtr(apiTask.PodID), ShouldEqual, utility.FromStringPtr(tc.at.PodID))

				So(utility.FromStringPtr(apiTask.Container), ShouldEqual, utility.FromStringPtr(tc.at.Container))
				So(apiTask.ContainerOpts.CPU, ShouldEqual, tc.at.ContainerOpts.CPU)
				So(apiTask.ContainerOpts.MemoryMB, ShouldEqual, tc.at.ContainerOpts.MemoryMB)
				So(utility.FromStringPtr(apiTask.ContainerOpts.WorkingDir), ShouldEqual, utility.FromStringPtr(tc.at.ContainerOpts.WorkingDir))
				So(utility.FromStringPtr(apiTask.ContainerOpts.Image), ShouldEqual, utility.FromStringPtr(tc.at.ContainerOpts.Image))
				So(utility.FromStringPtr(apiTask.ContainerOpts.OS), ShouldEqual, utility.FromStringPtr(tc.at.ContainerOpts.OS))
				So(utility.FromStringPtr(apiTask.ContainerOpts.Arch), ShouldEqual, utility.FromStringPtr(tc.at.ContainerOpts.Arch))
				So(apiTask.ContainerAllocated, ShouldEqual, tc.at.ContainerAllocated)
				So(apiTask.ContainerAllocationAttempts, ShouldEqual, tc.at.ContainerAllocationAttempts)

				if tc.at.StepbackInfo == nil {
					So(apiTask.StepbackInfo, ShouldBeNil)
				} else {
					So(apiTask.StepbackInfo, ShouldEqual, tc.at.StepbackInfo)
				}
			}
		})
	})
}
