//go:build !race
// +build !race

package cloud

import (
	"context"
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	running  = 16
	stopping = 64
	stopped  = 80
)

func fetchTestDistro() distro.Distro {
	return distro.Distro{
		Id:       "test_distro",
		Arch:     "linux_amd64",
		WorkDir:  "/data/mci",
		Provider: evergreen.ProviderNameEc2Spot,
		ProviderSettingsList: []*birch.Document{birch.NewDocument(
			birch.EC.String("ami", "ami-97785bed"),
			birch.EC.String("instance_type", "t2.micro"),
			birch.EC.String("key_name", "mci"),
			birch.EC.String("region", "us-east-1"),
			birch.EC.Boolean("is_vpc", true),
			birch.EC.Double("bid_price", .005),
			// TODO : The below settings require access to our staging environment. We
			// hope to make settings test data better in the future so that we do not
			// have to include values like these in our test code.
			birch.EC.String("vpc_name", "stage_dynamic_vpc"),
			birch.EC.String("subnet_id", "subnet-517c941a"),
			birch.EC.SliceString("security_group_ids", []string{"sg-601a6c13"}),
		)},
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 10,
		},
		SetupAsSudo: true,
		Setup:       "",
		User:        "root",
		SSHKey:      "",
	}
}

// This spawns (and terminates) an actual on-demand instance in AWS. Since AWS
// bills for a minimum of 1 minute, this will cost approximately $.0002 each
// time it is run (t1.micro $0.0116 per hour / 60 minutes). Since the price is
// low, we run this each time the integration cloud tests are run.
func TestSpawnEC2InstanceOnDemand(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	testConfig := env.Settings()

	testutil.ConfigureIntegrationTest(t, testConfig, t.Name())
	require.NoError(db.Clear(host.Collection))

	opts := &EC2ManagerOptions{
		client:   &awsClientImpl{},
		provider: onDemandProvider,
	}

	m := &ec2Manager{env: env, EC2ManagerOptions: opts}
	require.NoError(m.Configure(ctx, testConfig))
	require.NoError(m.client.Create(m.credentials, evergreen.DefaultEC2Region))

	d := fetchTestDistro()
	d.Provider = evergreen.ProviderNameEc2OnDemand
	h := host.NewIntent(host.CreateOptions{
		Distro:   d,
		UserName: evergreen.User,
		UserHost: false,
	})
	h, err := m.SpawnHost(ctx, h)
	assert.NoError(err)
	assert.NoError(h.Insert())
	foundHosts, err := host.Find(host.IsUninitialized)
	assert.NoError(err)
	assert.Len(foundHosts, 1)

	assert.NoError(m.TerminateInstance(ctx, h, evergreen.User, ""))
	foundHosts, err = host.Find(host.IsTerminated)
	assert.NoError(err)
	assert.Len(foundHosts, 1)

	instance, err := m.client.GetInstanceInfo(ctx, h.Id)
	assert.NoError(err)
	assert.NotContains([]int64{running, stopping, stopped}, *instance.State.Code)
}

func TestSpawnEC2InstanceSpot(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := testutil.NewEnvironment(ctx, t)
	testConfig := env.Settings()

	testutil.ConfigureIntegrationTest(t, testConfig, t.Name())
	require.NoError(db.Clear(host.Collection))
	opts := &EC2ManagerOptions{
		client:   &awsClientImpl{},
		provider: spotProvider,
	}

	m := &ec2Manager{env: env, EC2ManagerOptions: opts}
	require.NoError(m.Configure(ctx, testConfig))
	require.NoError(m.client.Create(m.credentials, evergreen.DefaultEC2Region))
	d := fetchTestDistro()
	d.Provider = evergreen.ProviderNameEc2Spot
	h := host.NewIntent(host.CreateOptions{
		Distro:   d,
		UserName: evergreen.User,
		UserHost: false,
	})
	h, err := m.SpawnHost(ctx, h)
	assert.NoError(err)
	assert.NoError(h.Insert())
	foundHosts, err := host.Find(host.IsUninitialized)
	assert.NoError(err)
	assert.Len(foundHosts, 1)
	assert.NoError(m.TerminateInstance(ctx, h, evergreen.User, ""))
	foundHosts, err = host.Find(host.IsTerminated)
	assert.NoError(err)
	assert.Len(foundHosts, 1)
}

func (s *EC2Suite) TestGetInstanceInfoFailsEarlyForSpotInstanceRequests() {
	opts := &EC2ManagerOptions{
		client:   &awsClientImpl{},
		provider: spotProvider,
	}
	m := &ec2Manager{env: s.env, EC2ManagerOptions: opts}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	info, err := m.client.GetInstanceInfo(ctx, "sir-123456")
	s.Nil(info)
	s.Errorf(err, "ID 'sir-123456' appears to be a spot instance request ID, not a host ID")
}

func (s *EC2Suite) TestGetInstanceInfoFailsEarlyForIntentHosts() {
	opts := &EC2ManagerOptions{
		client:   &awsClientImpl{},
		provider: onDemandProvider,
	}
	m := &ec2Manager{env: s.env, EC2ManagerOptions: opts}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	info, err := m.client.GetInstanceInfo(ctx, "evg-ubuntu-1234")
	s.Nil(info)
	s.Errorf(err, "intent host")
}
