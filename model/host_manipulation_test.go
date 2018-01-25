package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestHostDocumentConsistency(t *testing.T) {
	const hostName = "host1.test.10gen.cc"
	const staticProvider = "static"
	const secret = "iamasecret"
	const agentRevision = "12345"
	const distroName = "testStaticDistro"
	now := time.Now()

	testutil.ConfigureIntegrationTest(t, testutil.TestConfig(), "TestHostDocumentConsistency")
	assert := assert.New(t)

	staticTestDistro := &distro.Distro{
		Id:       distroName,
		Provider: evergreen.ProviderNameStatic,
		ProviderSettings: &map[string]interface{}{
			"hosts": []cloud.StaticHost{cloud.StaticHost{Name: hostName}},
		},
	}

	assert.NoError(db.Clear(distro.Collection))
	assert.NoError(db.Clear(host.Collection))
	assert.NoError(staticTestDistro.Insert())

	referenceHost := &host.Host{
		Id:                    hostName,
		Host:                  hostName,
		Distro:                *staticTestDistro,
		Provider:              staticProvider,
		CreationTime:          now,
		Secret:                secret,
		AgentRevision:         agentRevision,
		LastCommunicationTime: now,
	}
	assert.NoError(referenceHost.Insert())
	assert.NoError(UpdateStaticHosts())

	hostFromDB, err := host.FindOne(host.ById(hostName))
	assert.NoError(err)
	assert.NotNil(hostFromDB)

	assert.Equal(hostName, hostFromDB.Id)
	assert.Equal(hostName, hostFromDB.Host)
	assert.Equal(staticProvider, hostFromDB.Provider)
	assert.Equal(distroName, hostFromDB.Distro.Id)
	assert.Equal(secret, hostFromDB.Secret)
	assert.Equal(agentRevision, hostFromDB.AgentRevision)
	assert.WithinDuration(now, hostFromDB.LastCommunicationTime, 1*time.Millisecond)
	assert.False(hostFromDB.UserHost)

	// test that upserting a host does not clear out fields not set by UpdateStaticHosts
	const staticHostName = "staticHost"
	staticReferenceHost := host.Host{
		Id:           staticHostName,
		User:         "user",
		Host:         staticHostName,
		Distro:       *staticTestDistro,
		CreationTime: time.Now(),
		Provider:     evergreen.HostTypeStatic,
		StartedBy:    evergreen.User,
		Status:       evergreen.HostRunning,
		Provisioned:  true,
	}
	staticTestHost := staticReferenceHost
	staticReferenceHost.Secret = "secret"
	staticReferenceHost.LastCommunicationTime = time.Now()
	staticReferenceHost.AgentRevision = "agent_rev"
	assert.NoError(staticReferenceHost.Insert())
	_, err = staticTestHost.Upsert()
	assert.NoError(err)
	hostFromDB, err = host.FindOne(host.ById(staticHostName))
	assert.NoError(err)
	assert.NotNil(hostFromDB)
	assert.Equal(staticHostName, hostFromDB.Id)
	assert.Equal(staticReferenceHost.Secret, hostFromDB.Secret)
	assert.WithinDuration(staticReferenceHost.LastCommunicationTime, hostFromDB.LastCommunicationTime, 1*time.Second)
	assert.Equal(staticReferenceHost.AgentRevision, hostFromDB.AgentRevision)
}
