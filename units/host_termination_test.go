package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
)

func init() {
	if !util.StringSliceContains(evergreen.ProviderSpawnable, evergreen.ProviderNameMock) {
		evergreen.ProviderSpawnable = append(evergreen.ProviderSpawnable, evergreen.ProviderNameMock)
	}

}

func TestTerminateHosts(t *testing.T) {
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestTerminateHosts")
	assert := assert.New(t)
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	testutil.HandleTestingErr(db.Clear(host.Collection), t, "error clearing host collection")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	assert.NoError(env.Configure(ctx, "", nil))
	assert.NoError(env.Local.Start(ctx))

	hostID := "i-12345"
	mcp := cloud.GetMockProvider()
	mcp.Set(hostID, cloud.MockInstance{
		IsUp:   true,
		Status: cloud.StatusRunning,
	})

	// test that trying to terminate a host that does not exist is handled gracecfully
	h := &host.Host{
		Id:          hostID,
		Status:      evergreen.HostRunning,
		Provider:    evergreen.ProviderNameMock,
		Provisioned: true,
	}
	assert.NoError(h.Insert())
	j := NewHostTerminationJob(env, *h)
	j.Run(ctx)

	assert.NoError(j.Error())
	dbHost, err := host.FindOne(host.ById(h.Id))
	assert.NoError(err)
	assert.NotNil(dbHost)
	assert.Equal(evergreen.HostTerminated, dbHost.Status)
}
