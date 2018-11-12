package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
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
					Id: ToAPIString("testId"),
					Distro: DistroInfo{
						Id:       ToAPIString("testDistroId"),
						Provider: ToAPIString("testDistroProvider"),
					},
					Provisioned: true,
					StartedBy:   ToAPIString("testStarter"),
					Type:        ToAPIString("testType"),
					User:        ToAPIString("testUser"),
					Status:      ToAPIString("testStatus"),
					RunningTask: taskInfo{
						Id:           ToAPIString("testRunningTaskId"),
						Name:         ToAPIString("testRTName"),
						DispatchTime: NewTime(timeNow),
						VersionId:    ToAPIString("testVersionId"),
						BuildId:      ToAPIString("testBuildId"),
					},
				},
				sh: host.Host{
					Id: "testId",
					Distro: distro.Distro{
						Id:       "testDistroId",
						Provider: evergreen.ProviderNameMock,
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
				sh: host.Host{
					Distro: distro.Distro{
						Provider: evergreen.ProviderNameMock,
					},
				},
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
				So(FromAPIString(apiHost.Id), ShouldEqual, FromAPIString(hc.ah.Id))
				So(FromAPIString(apiHost.Status), ShouldEqual, FromAPIString(hc.ah.Status))
			}
		})
	})
}
