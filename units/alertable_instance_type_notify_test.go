package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type AlertableInstanceTypeNotifySuite struct {
	suite.Suite
	env evergreen.Environment
	ctx context.Context
}

func TestAlertableInstanceTypeNotifySuite(t *testing.T) {
	suite.Run(t, new(AlertableInstanceTypeNotifySuite))
}

func (s *AlertableInstanceTypeNotifySuite) SetupSuite() {
	s.ctx = context.Background()
	s.env = testutil.NewEnvironment(s.ctx, s.T())
}

func (s *AlertableInstanceTypeNotifySuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(host.Collection, user.Collection, evergreen.ConfigCollection))
}

func (s *AlertableInstanceTypeNotifySuite) TestJobWithNoAlertableTypes() {
	// Set up config with no alertable instance types
	settings := &evergreen.Settings{
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				AlertableInstanceTypes: []string{},
			},
		},
	}
	s.Require().NoError(settings.Set(s.ctx))

	job := NewAlertableInstanceTypeNotifyJob("test-id").(*alertableInstanceTypeNotifyJob)
	job.env = s.env
	job.Run(s.ctx)

	s.NoError(job.Error())
}

func (s *AlertableInstanceTypeNotifySuite) TestJobWithNoMatchingHosts() {
	// Set up config with alertable instance types
	settings := &evergreen.Settings{
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				AlertableInstanceTypes: []string{"m5.large", "c5.xlarge"},
			},
		},
	}
	s.Require().NoError(settings.Set(s.ctx))

	// Create a spawn host with a non-alertable instance type
	h := &host.Host{
		Id:           "test-host-1",
		UserHost:     true,
		StartedBy:    "test-user",
		Status:       evergreen.HostRunning,
		InstanceType: "t3.micro",
		Provider:     evergreen.ProviderNameEc2OnDemand,
		Distro: distro.Distro{
			Id: "test-distro",
		},
	}
	s.Require().NoError(h.Insert(s.ctx))

	job := NewAlertableInstanceTypeNotifyJob("test-id").(*alertableInstanceTypeNotifyJob)
	job.env = s.env
	job.Run(s.ctx)

	s.NoError(job.Error())
}

func (s *AlertableInstanceTypeNotifySuite) TestJobWithMatchingHosts() {
	// Set up config with alertable instance types
	settings := &evergreen.Settings{
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				AlertableInstanceTypes: []string{"m5.large", "c5.xlarge"},
			},
		},
	}
	s.Require().NoError(settings.Set(s.ctx))

	// Create a user
	u := &user.DBUser{
		Id:           "test-user",
		EmailAddress: "test@example.com",
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "U123456",
		},
	}
	s.Require().NoError(u.Insert(s.ctx))

	// Create spawn hosts with alertable instance types
	h1 := &host.Host{
		Id:           "test-host-1",
		UserHost:     true,
		StartedBy:    "test-user",
		Status:       evergreen.HostRunning,
		InstanceType: "m5.large",
		Provider:     evergreen.ProviderNameEc2OnDemand,
		Distro: distro.Distro{
			Id: "test-distro",
		},
	}
	s.Require().NoError(h1.Insert(s.ctx))

	h2 := &host.Host{
		Id:           "test-host-2",
		UserHost:     true,
		StartedBy:    "test-user",
		Status:       evergreen.HostRunning,
		InstanceType: "c5.xlarge",
		Provider:     evergreen.ProviderNameEc2OnDemand,
		Distro: distro.Distro{
			Id: "test-distro",
		},
	}
	s.Require().NoError(h2.Insert(s.ctx))

	// Create a non-user host (should be ignored)
	h3 := &host.Host{
		Id:           "test-host-3",
		UserHost:     false,
		StartedBy:    evergreen.User,
		Status:       evergreen.HostRunning,
		InstanceType: "m5.large",
		Provider:     evergreen.ProviderNameEc2OnDemand,
		Distro: distro.Distro{
			Id: "test-distro",
		},
	}
	s.Require().NoError(h3.Insert(s.ctx))

	job := NewAlertableInstanceTypeNotifyJob("test-id").(*alertableInstanceTypeNotifyJob)
	job.env = s.env
	job.Run(s.ctx)

	s.NoError(job.Error())
}

func (s *AlertableInstanceTypeNotifySuite) TestBuildEmailBody() {
	hosts := []host.Host{
		{
			Id:           "host-1",
			InstanceType: "m5.large",
			Status:       evergreen.HostRunning,
		},
		{
			Id:           "host-2",
			InstanceType: "c5.xlarge",
			Status:       evergreen.HostStopped,
		},
	}

	job := &alertableInstanceTypeNotifyJob{}
	body := job.buildEmailBody(hosts)

	s.Contains(body, "host-1")
	s.Contains(body, "m5.large")
	s.Contains(body, "host-2")
	s.Contains(body, "c5.xlarge")
}

func (s *AlertableInstanceTypeNotifySuite) TestBuildSlackMessage() {
	hosts := []host.Host{
		{
			Id:           "host-1",
			InstanceType: "m5.large",
			Status:       evergreen.HostRunning,
		},
	}

	job := &alertableInstanceTypeNotifyJob{}
	msg := job.buildSlackMessage(hosts)

	s.Contains(msg, "host-1")
	s.Contains(msg, "m5.large")
}

func (s *AlertableInstanceTypeNotifySuite) TestJobWithCorrectHostFiltering() {
	// Set up config with alertable instance types
	settings := &evergreen.Settings{
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				AlertableInstanceTypes: []string{"m5.large"},
			},
		},
	}
	s.Require().NoError(settings.Set(s.ctx))

	// Create a user
	u := &user.DBUser{
		Id:           "test-user",
		EmailAddress: "test@example.com",
	}
	s.Require().NoError(u.Insert(s.ctx))

	// Create hosts with different configurations
	hosts := []host.Host{
		// User spawn host with alertable instance type - should trigger alert
		{
			Id:                   "user-spawn-alertable",
			UserHost:             true,
			StartedBy:            "test-user",
			Status:               evergreen.HostRunning,
			InstanceType:         "m5.large",
			CreationTime:         time.Now().Add(-4 * 24 * time.Hour), // 4 days old
			LastInstanceEditTime: time.Now().Add(-4 * 24 * time.Hour), // 4 days since edit
		},
		// Task host with alertable instance type - should NOT trigger alert
		{
			Id:           "task-host-alertable",
			UserHost:     false,
			StartedBy:    evergreen.User,
			Status:       evergreen.HostRunning,
			InstanceType: "m5.large",
			CreationTime: time.Now().Add(-4 * 24 * time.Hour),
		},
		// User spawn host with non-alertable instance type - should NOT trigger alert
		{
			Id:           "user-spawn-non-alertable",
			UserHost:     true,
			StartedBy:    "test-user",
			Status:       evergreen.HostRunning,
			InstanceType: "t3.micro",
			CreationTime: time.Now().Add(-4 * 24 * time.Hour),
		},
		// Terminated user spawn host with alertable instance type - should NOT trigger alert
		{
			Id:           "user-spawn-terminated",
			UserHost:     true,
			StartedBy:    "test-user",
			Status:       evergreen.HostTerminated,
			InstanceType: "m5.large",
			CreationTime: time.Now().Add(-4 * 24 * time.Hour),
		},
	}

	for _, h := range hosts {
		s.Require().NoError(h.Insert(s.ctx))
	}

	job := NewAlertableInstanceTypeNotifyJob("test-id").(*alertableInstanceTypeNotifyJob)
	job.env = s.env
	job.Run(s.ctx)

	s.NoError(job.Error())
}

func TestNewAlertableInstanceTypeNotifyJob(t *testing.T) {
	job := NewAlertableInstanceTypeNotifyJob("test-id")
	assert.NotNil(t, job)
	assert.Equal(t, "alertable-instance-type-notify.test-id", job.ID())
	assert.Equal(t, alertableInstanceTypeNotifyJobName, job.Type().Name)
}
