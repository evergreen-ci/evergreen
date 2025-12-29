package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestHostMonitoringContainerStateJob(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(db.Clear(host.Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	h1 := &host.Host{
		Id:            "parent-1",
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	h2 := &host.Host{
		Id:       "container-1",
		Status:   evergreen.HostRunning,
		ParentID: "parent-1",
	}
	h3 := &host.Host{
		Id:       "container-2",
		Status:   evergreen.HostRunning,
		ParentID: "parent-1",
	}
	h4 := &host.Host{
		Id:       "container-3",
		Status:   evergreen.HostUninitialized,
		ParentID: "parent-1",
	}
	assert.NoError(h1.Insert(ctx))
	assert.NoError(h2.Insert(ctx))
	assert.NoError(h3.Insert(ctx))
	assert.NoError(h4.Insert(ctx))

	j := NewHostMonitorContainerStateJob(h1, evergreen.ProviderNameDockerMock, "job-1")
	assert.False(j.Status().Completed)

	j.Run(ctx)

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

	container1, err := host.FindOne(ctx, host.ById("container-1"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, container1.Status)

	container2, err := host.FindOne(ctx, host.ById("container-2"))
	assert.NoError(err)
	assert.Equal(evergreen.HostTerminated, container2.Status)
	// Verify termination_time is set (DEVPROD-25714)
	assert.False(container2.TerminationTime.IsZero(), "termination_time should be set when container is terminated")

	container3, err := host.FindOne(ctx, host.ById("container-3"))
	assert.NoError(err)
	assert.Equal(evergreen.HostUninitialized, container3.Status)
}

// TestHostMonitoringContainerStateTerminationTime verifies termination_time is
// set when containers not in Docker are terminated.
func TestHostMonitoringContainerStateTerminationTime(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(host.Collection))

	parent := &host.Host{Id: "parent-test", Status: evergreen.HostRunning, HasContainers: true}
	decommissioned := &host.Host{Id: "decommissioned", Status: evergreen.HostDecommissioned, ParentID: "parent-test"}
	running := &host.Host{Id: "running", Status: evergreen.HostRunning, ParentID: "parent-test"}

	assert.NoError(parent.Insert(t.Context()))
	assert.NoError(decommissioned.Insert(t.Context()))
	assert.NoError(running.Insert(t.Context()))

	j := NewHostMonitorContainerStateJob(parent, evergreen.ProviderNameDockerMock, "test")
	j.Run(t.Context())
	assert.NoError(j.Error())

	dbContainer1, err := host.FindOne(t.Context(), host.ById("decommissioned"))
	assert.NoError(err)
	assert.Equal(evergreen.HostTerminated, dbContainer1.Status)
	assert.False(dbContainer1.TerminationTime.IsZero(), "termination_time must be set")

	dbContainer2, err := host.FindOne(t.Context(), host.ById("running"))
	assert.NoError(err)
	assert.Equal(evergreen.HostTerminated, dbContainer2.Status)
	assert.False(dbContainer2.TerminationTime.IsZero(), "termination_time must be set")
}
