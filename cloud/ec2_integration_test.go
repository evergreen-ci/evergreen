//go:build !race
// +build !race

package cloud

import (
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fetchTestDistro() distro.Distro {
	return distro.Distro{
		Id:       "test_distro",
		Arch:     "linux_amd64",
		WorkDir:  "/data/mci",
		Provider: evergreen.ProviderNameEc2Fleet,
		ProviderSettingsList: []*birch.Document{birch.NewDocument(
			birch.EC.String("ami", "ami-97785bed"),
			birch.EC.String("instance_type", "t2.micro"),
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
	}
}

// This spawns (and terminates) an actual EC2 instance in AWS via CreateFleet.
// Since AWS bills for a minimum of 1 minute, this will cost approximately
// $.0002 each time it is run (t1.micro $0.0116 per hour / 60 minutes). Since
// the price is low, we run this each time the integration cloud tests are run.
func TestSpawnEC2InstanceFleet(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	ctx := t.Context()
	env := testutil.NewEnvironment(ctx, t)
	testConfig := env.Settings()
	testConfig.SSH.TaskHostKey.Name = "evergreen-task-hosts"

	testutil.ConfigureIntegrationTest(t, testConfig)
	// ec2FleetManager.makeOverrides requires at least one subnet in the global
	// AWS settings. Populate it with the distro's subnet so the fleet manager
	// can resolve overrides without error.
	testConfig.Providers.AWS.Subnets = []evergreen.Subnet{
		{AZ: "us-east-1a", SubnetID: "subnet-517c941a"},
	}
	require.NoError(db.Clear(host.Collection))

	m := &ec2FleetManager{
		env: env,
		EC2FleetManagerOptions: &EC2FleetManagerOptions{
			client: &awsClientImpl{},
		},
	}
	require.NoError(m.Configure(ctx, testConfig))

	d := fetchTestDistro()
	h := host.NewIntent(host.CreateOptions{
		Distro:   d,
		UserName: evergreen.User,
		UserHost: false,
		// The assumed role used for this integration test requires
		// a specific resource tag to be set in order to perform
		// certain actions (e.g., terminate a host). This means that
		// the resource must have the tag or the action cannot be
		// performed against it.
		InstanceTags: []host.Tag{
			{
				Key:   "evergreen-integration-testing",
				Value: "true",
			},
		},
	})
	h, err := m.SpawnHost(ctx, h)
	require.NoError(err)
	require.NotNil(h)
	assert.NoError(h.Insert(ctx))
	foundHosts, err := host.Find(ctx, host.IsUninitialized)
	assert.NoError(err)
	require.Len(foundHosts, 1)

	assert.NoError(m.TerminateInstance(ctx, h, evergreen.User, ""))
	foundHosts, err = host.Find(ctx, host.IsTerminated)
	assert.NoError(err)
	require.Len(foundHosts, 1)

	// AWS state propagation for fleet-launched instances can lag after TerminateInstance,
	// so poll until DescribeInstances reflects the terminated state.
	require.Eventually(func() bool {
		info, err := m.GetInstanceState(ctx, h)
		if err != nil {
			return false
		}
		return info.Status != StatusRunning && info.Status != StatusStopping && info.Status != StatusStopped
	}, 60*time.Second, 2*time.Second, "AWS should reflect terminated state after TerminateInstance")
}
