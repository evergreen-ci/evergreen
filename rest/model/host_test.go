package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
					Id: utility.ToStringPtr("testId"),
					Distro: DistroInfo{
						Id:       utility.ToStringPtr("testDistroId"),
						Provider: utility.ToStringPtr("testDistroProvider"),
					},
					Provisioned:  true,
					StartedBy:    utility.ToStringPtr("testStarter"),
					InstanceType: utility.ToStringPtr("testType"),
					User:         utility.ToStringPtr("testUser"),
					Status:       utility.ToStringPtr("testStatus"),
					RunningTask: TaskInfo{
						Id:           utility.ToStringPtr("testRunningTaskId"),
						Name:         utility.ToStringPtr("testRTName"),
						DispatchTime: &timeNow,
						VersionId:    utility.ToStringPtr("testVersionId"),
						BuildId:      utility.ToStringPtr("testBuildId"),
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
					RunningTask: TaskInfo{
						DispatchTime: &time.Time{},
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
				apiHost.BuildFromService(&hc.sh, &hc.st)
				So(utility.FromStringPtr(apiHost.Id), ShouldEqual, utility.FromStringPtr(hc.ah.Id))
				So(utility.FromStringPtr(apiHost.Status), ShouldEqual, utility.FromStringPtr(hc.ah.Status))
			}
		})
	})

	t.Run("BuildFromServicePopulatesProvisionOptionsForHostSpawnedFromTask", func(t *testing.T) {
		h := host.Host{
			Id: "host_id",
			ProvisionOptions: &host.ProvisionOptions{
				TaskId: "task_id",
			},
		}
		var apiHost APIHost
		apiHost.BuildFromService(&h, nil)
		assert.Equal(t, h.Id, utility.FromStringPtr(apiHost.Id))
		require.NotZero(t, apiHost.ProvisionOptions)
		assert.Equal(t, h.ProvisionOptions.TaskId, utility.FromStringPtr(apiHost.ProvisionOptions.TaskID))
	})
}
