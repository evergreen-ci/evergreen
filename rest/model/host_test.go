package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
)

type hostCompare struct {
	ah APIHost
	sh host.Host
	st task.Task
}

func TestHostBuildFromService(t *testing.T) {
	Convey("With list of model pairs", t, func() {
		timeNow := time.Now()
		// List of hosts to compare. Should make adding hosts in the future easy
		// if some case is not present.
		modelPairs := []hostCompare{
			{
				ah: APIHost{
					Id: ToApiString("testId"),
					Distro: DistroInfo{
						Id:       ToApiString("testDistroId"),
						Provider: ToApiString("testDistroProvider"),
					},
					Provisioned: true,
					StartedBy:   ToApiString("testStarter"),
					Type:        ToApiString("testType"),
					User:        ToApiString("testUser"),
					Status:      ToApiString("testStatus"),
					RunningTask: taskInfo{
						Id:           ToApiString("testRunningTaskId"),
						Name:         ToApiString("testRTName"),
						DispatchTime: NewTime(timeNow),
						VersionId:    ToApiString("testVersionId"),
						BuildId:      ToApiString("testBuildId"),
					},
				},
				sh: host.Host{
					Id: "testId",
					Distro: distro.Distro{
						Id:       "testDistroId",
						Provider: "testDistroProvider",
					},
					Provisioned:  true,
					StartedBy:    "testStarter",
					InstanceType: "testType",
					User:         "testUser",
					Status:       "testStatus",
				},
				st: task.Task{
					Id:           "testRunningTaskId",
					DisplayName:  "testRTName",
					DispatchTime: timeNow,
					Version:      "testVersionId",
					BuildId:      "testBuildId",
				},
			},
			// Empty on purpose to ensure zero values are correctly converted
			{
				ah: APIHost{
					RunningTask: taskInfo{
						DispatchTime: NewTime(time.Time{}),
					},
				},
				sh: host.Host{},
				st: task.Task{},
			},
		}
		Convey("running BuildFromService() should produce the equivalent model", func() {
			for _, hc := range modelPairs {
				apiHost := &APIHost{}
				err := apiHost.BuildFromService(hc.st)
				So(err, ShouldBeNil)
				err = apiHost.BuildFromService(hc.sh)
				So(err, ShouldBeNil)
				So(FromApiString(apiHost.Id), ShouldEqual, FromApiString(hc.ah.Id))
				So(FromApiString(apiHost.Status), ShouldEqual, FromApiString(hc.ah.Status))
			}
		})
	})
}
