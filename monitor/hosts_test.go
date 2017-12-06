package monitor

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestTerminateHosts(t *testing.T) {
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestTerminateHosts")
	assert := assert.New(t)
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	testutil.HandleTestingErr(db.Clear(host.Collection), t, "error clearing host collection")
	ctx := context.Background()
	reason := "test"

	// test that trying to terminate a host that does not exist is handled gracecfully
	h := host.Host{
		Id:       "i-12345",
		Status:   evergreen.HostRunning,
		Provider: evergreen.ProviderNameEc2OnDemand,
	}
	assert.NoError(h.Insert())

	assert.NoError(terminateHosts(ctx, []host.Host{h}, testConfig, reason))
	dbHost, err := host.FindOne(host.ById(h.Id))
	assert.NoError(err)
	assert.NotNil(dbHost)
	assert.Equal(evergreen.HostTerminated, dbHost.Status)
}
