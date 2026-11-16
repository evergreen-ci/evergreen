package units

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
)

func TestGetHostTerminationReason(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)

	for tName, tCase := range map[string]struct {
		h        host.Host
		expected string
	}{
		"BuildingFailedHostShouldReportCreationFailure": {
			h:        host.Host{Status: evergreen.HostBuildingFailed, StartedBy: evergreen.User},
			expected: "host failed to be created in the cloud provider",
		},
		"ProvisionFailedHostShouldReportProvisionFailure": {
			h:        host.Host{Status: evergreen.HostProvisionFailed, StartedBy: evergreen.User},
			expected: "host failed to provision",
		},
		"DecommissionedIdleHostShouldReportNotRunningTask": {
			h:        host.Host{Status: evergreen.HostDecommissioned, StartedBy: evergreen.User, Provisioned: true},
			expected: "host was decommissioned and is not running a task",
		},
		"DecommissionedIdleSingleTaskHostShouldReportFinishedTask": {
			h: host.Host{
				Status:    evergreen.HostDecommissioned,
				StartedBy: evergreen.User,
				LastTask:  "t1",
				Distro:    distro.Distro{SingleTaskDistro: true},
			},
			expected: "single task host finished its task",
		},
		"DecommissionedUnreachableHostRunningTaskShouldReportLastCommunication": {
			h: host.Host{
				Status:                evergreen.HostDecommissioned,
				StartedBy:             evergreen.User,
				Provisioned:           true,
				RunningTask:           "t1",
				LastCommunicationTime: now.Add(-time.Hour),
				ExpirationTime:        future,
			},
			expected: "host was decommissioned and has not communicated with Evergreen since 2026-10-28T11:00:00Z",
		},
		"DecommissionedUnreachableHostWithReasonShouldReportDecommissionReason": {
			h: host.Host{
				Status:                evergreen.HostDecommissioned,
				DecommissionReason:    "SNS notification indicates host is stopped",
				StartedBy:             evergreen.User,
				Provisioned:           true,
				RunningTask:           "t1",
				LastCommunicationTime: now.Add(-time.Hour),
				ExpirationTime:        future,
			},
			expected: "host was decommissioned (SNS notification indicates host is stopped) and has not communicated with Evergreen since 2026-10-28T11:00:00Z",
		},
		"DecommissionedHostRunningTaskThatNeverCommunicatedShouldReportNeverCommunicated": {
			h: host.Host{
				Status:      evergreen.HostDecommissioned,
				StartedBy:   evergreen.User,
				Provisioned: true,
				RunningTask: "t1",
			},
			expected: "host was decommissioned and has never communicated with Evergreen",
		},
		"ExpiredSpawnHostShouldReportExpiration": {
			h: host.Host{
				Status:         evergreen.HostRunning,
				StartedBy:      "user",
				Provisioned:    true,
				UserHost:       true,
				ExpirationTime: now.Add(-time.Minute),
			},
			expected: "host has passed its expiration time",
		},
		"UnprovisionedHostShouldReportProvisioningTimeout": {
			h: host.Host{
				Status:    evergreen.HostProvisioning,
				StartedBy: evergreen.User,
			},
			expected: "host took too long to provision",
		},
		"UnmatchedHostShouldReportGenericReason": {
			h: host.Host{
				Status:                evergreen.HostDecommissioned,
				StartedBy:             evergreen.User,
				Provisioned:           true,
				RunningTask:           "t1",
				LastCommunicationTime: now,
			},
			expected: "host matched termination criteria",
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.Equal(t, tCase.expected, getHostTerminationReason(&tCase.h, now))
		})
	}
}
