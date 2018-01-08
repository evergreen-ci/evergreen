package cloud

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpotInstanceIntegration(t *testing.T) {
	assert := assert.New(t)   // nolint
	require := require.New(t) // nolint

	testConfig = testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	testutil.ConfigureIntegrationTest(t, testConfig, "TestSpawnSpotInstance")
	require.NoError(db.Clear(host.Collection))
	opts := &EC2ManagerOptions{
		client: &awsClientImpl{},
	}
	m := NewEC2Manager(opts).(*ec2Manager)
	require.NoError(m.Configure(testConfig))
	require.NoError(m.client.Create(m.credentials))
	d := fetchTestDistro()
	h := NewIntent(*d, m.GetInstanceName(d), d.Provider, HostOptions{
		UserName: evergreen.User,
		UserHost: false,
	})
	h, err := m.SpawnHost(h)
	assert.NoError(err)
	assert.NoError(h.Insert())
	foundHosts, err := host.Find(host.IsUninitialized)
	assert.NoError(err)
	assert.Len(foundHosts, 1)
	assert.NoError(m.TerminateInstance(h))
	foundHosts, err = host.Find(host.IsTerminated)
	assert.NoError(err)
	assert.Len(foundHosts, 1)
}

func fetchTestDistro() *distro.Distro {
	return &distro.Distro{
		Id:       "test_distro",
		Arch:     "linux_amd64",
		WorkDir:  "/data/mci",
		PoolSize: 10,
		Provider: evergreen.ProviderNameEc2Spot,
		ProviderSettings: &map[string]interface{}{
			"ami":            "ami-c7e7f2d0",
			"instance_type":  "t1.micro",
			"key_name":       "mci",
			"bid_price":      .005,
			"security_group": "default",
		},

		SetupAsSudo: true,
		Setup:       "",
		Teardown:    "",
		User:        "root",
		SSHKey:      "",
	}
}
