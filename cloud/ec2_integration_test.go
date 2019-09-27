// +build !race

package cloud

import (
	"context"
	"testing"

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
		PoolSize: 10,
		Provider: evergreen.ProviderNameEc2Spot,
		ProviderSettings: &map[string]interface{}{
			"ami":           "ami-97785bed",
			"instance_type": "t2.micro",
			"key_name":      "mci",
			"bid_price":     .005,
			"is_vpc":        true,
			// TODO : The below settings require access to our staging environment. We
			// hope to make settings test data better in the future so that we do not
			// have to include values like these in our test code.
			"security_group_ids": []string{"sg-601a6c13"},
			"subnet_id":          "subnet-517c941a",
			"vpc_name":           "stage_dynamic_vpc",
		},

		SetupAsSudo: true,
		Setup:       "",
		Teardown:    "",
		User:        "root",
		SSHKey:      "",
	}
}

// This spawns (and terminates) an actual on-demand instance in AWS. Since AWS bills for a minimum
// of 1 minute, this will cost approximately $.0002 each time it is run (t1.micro
// $0.0116 per hour / 60 minutes). Since the price is low, we run this each time
// the integration cloud tests are run.
func TestSpawnEC2InstanceOnDemand(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	testConfig := env.Settings()

	testutil.ConfigureIntegrationTest(t, testConfig, "TestSpawnEC2Instance")
	require.NoError(db.Clear(host.Collection))

	opts := &EC2ManagerOptions{
		client:   &awsClientImpl{},
		provider: onDemandProvider,
	}

	m := NewEC2Manager(opts).(*ec2Manager)
	require.NoError(m.Configure(ctx, testConfig))
	require.NoError(m.client.Create(m.credentials, defaultRegion))

	d := fetchTestDistro()
	d.Provider = evergreen.ProviderNameEc2OnDemand
	h := host.NewIntent(d, d.GenerateName(), d.Provider, host.CreateOptions{
		UserName: evergreen.User,
		UserHost: false,
	})
	h, err := m.SpawnHost(ctx, h)
	assert.NoError(err)
	assert.NoError(h.Insert())
	foundHosts, err := host.Find(host.IsUninitialized)
	assert.NoError(err)
	assert.Len(foundHosts, 1)
	assert.NoError(m.OnUp(ctx, h))

	assert.NoError(m.TerminateInstance(ctx, h, evergreen.User))
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

	testutil.ConfigureIntegrationTest(t, testConfig, "TestSpawnSpotInstance")
	require.NoError(db.Clear(host.Collection))
	opts := &EC2ManagerOptions{
		client:   &awsClientImpl{},
		provider: spotProvider,
	}

	m := NewEC2Manager(opts).(*ec2Manager)
	require.NoError(m.Configure(ctx, testConfig))
	require.NoError(m.client.Create(m.credentials, defaultRegion))
	d := fetchTestDistro()
	d.Provider = evergreen.ProviderNameEc2Spot
	h := host.NewIntent(d, d.GenerateName(), d.Provider, host.CreateOptions{
		UserName: evergreen.User,
		UserHost: false,
	})
	h, err := m.SpawnHost(ctx, h)
	assert.NoError(err)
	assert.NoError(h.Insert())
	foundHosts, err := host.Find(host.IsUninitialized)
	assert.NoError(err)
	assert.Len(foundHosts, 1)
	assert.NoError(m.TerminateInstance(ctx, h, evergreen.User))
	foundHosts, err = host.Find(host.IsTerminated)
	assert.NoError(err)
	assert.Len(foundHosts, 1)
}

func (s *EC2Suite) TestGetInstanceInfoFailsEarlyForSpotInstanceRequests() {
	opts := &EC2ManagerOptions{
		client:   &awsClientImpl{},
		provider: spotProvider,
	}
	m := NewEC2Manager(opts).(*ec2Manager)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	info, err := m.client.GetInstanceInfo(ctx, "sir-123456")
	s.Nil(info)
	s.Errorf(err, "id appears to be a spot instance request ID, not a host ID (\"sir-123456\")")
}
