package model

import (
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
)

func TestStaticHostDocumentConsistency(t *testing.T) {
	assert := assert.New(t)
	const hostName = "hostName"
	const secret = "iamasecret"
	const agentRevision = "12345"
	const distroName = "testStaticDistro"
	const numHosts = 100
	const numDistros = 10
	const hostsPerDistro = numHosts / numDistros
	now := time.Now()

	assert.NoError(db.Clear(distro.Collection))
	assert.NoError(db.Clear(host.Collection))

	// test 100 hosts in 10 distros to make sure they match afterwards
	for i := 0; i < numHosts; i++ {
		hostInterval := strconv.Itoa(i)
		distroInterval := strconv.Itoa(i / hostsPerDistro)
		hosts := []cloud.StaticHost{}
		for j := (i / hostsPerDistro * hostsPerDistro); j < ((i/hostsPerDistro + 1) * hostsPerDistro); j++ {
			hosts = append(hosts, cloud.StaticHost{Name: hostName + strconv.Itoa(j)})
		}
		shuffledHosts := []cloud.StaticHost{}
		for _, idx := range rand.Perm(len(hosts)) {
			shuffledHosts = append(shuffledHosts, hosts[idx])
		}
		staticTestDistro := distro.Distro{
			Id:       distroName + distroInterval,
			Provider: evergreen.ProviderNameStatic,
			ProviderSettings: &map[string]interface{}{
				"hosts": shuffledHosts,
			},
		}
		_ = staticTestDistro.Insert()
		referenceHost := &host.Host{
			Id:                    hostName + hostInterval,
			Host:                  hostName + hostInterval,
			Distro:                staticTestDistro,
			Provider:              evergreen.HostTypeStatic,
			CreationTime:          now,
			Secret:                secret,
			AgentRevision:         agentRevision,
			LastCommunicationTime: now,
		}
		assert.NoError(referenceHost.Insert())
	}

	assert.NoError(UpdateStaticHosts())

	for i := 0; i < numHosts; i++ {
		hostInterval := strconv.Itoa(i)
		distroInterval := strconv.Itoa(i / hostsPerDistro)
		hostFromDB, err := host.FindOne(host.ById(hostName + hostInterval))
		assert.NoError(err)
		assert.NotNil(hostFromDB)
		assert.Equal(hostName+hostInterval, hostFromDB.Id)
		assert.Equal(hostName+hostInterval, hostFromDB.Host)
		assert.Equal(evergreen.HostTypeStatic, hostFromDB.Provider)
		assert.Equal(distroName+distroInterval, hostFromDB.Distro.Id)
		assert.Equal(secret, hostFromDB.Secret)
		assert.Equal(agentRevision, hostFromDB.AgentRevision)
		assert.WithinDuration(now, hostFromDB.LastCommunicationTime, 1*time.Millisecond)
		assert.False(hostFromDB.UserHost)
	}

	// test that upserting a host does not clear out fields not set by UpdateStaticHosts
	const staticHostName = "staticHost"
	staticTestDistro := &distro.Distro{
		Id:       distroName,
		Provider: evergreen.ProviderNameStatic,
		ProviderSettings: &map[string]interface{}{
			"hosts": []cloud.StaticHost{cloud.StaticHost{Name: hostName}},
		},
	}
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
	_, err := staticTestHost.Upsert()
	assert.NoError(err)
	hostFromDB, err := host.FindOne(host.ById(staticHostName))
	assert.NoError(err)
	assert.NotNil(hostFromDB)
	assert.Equal(staticHostName, hostFromDB.Id)
	assert.Equal(staticReferenceHost.Secret, hostFromDB.Secret)
	assert.WithinDuration(staticReferenceHost.LastCommunicationTime, hostFromDB.LastCommunicationTime, 1*time.Second)
	assert.Equal(staticReferenceHost.AgentRevision, hostFromDB.AgentRevision)
}

func TestDockerStaticHostUpdate(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(distro.Collection))
	assert.NoError(db.Clear(host.Collection))

	const hostName = "hostName"
	const agentRevision = "12345"
	const distroName = "distroName"
	const numHosts = 10

	hosts := []cloud.StaticHost{}

	for i := 0; i < numHosts; i++ {
		hostInterval := strconv.Itoa(i)
		hosts = append(hosts, cloud.StaticHost{Name: hostName + hostInterval})
	}

	d := distro.Distro{
		Id:       distroName,
		Provider: evergreen.ProviderNameDockerStatic,
		ProviderSettings: &map[string]interface{}{
			"hosts": hosts,
		},
	}
	assert.NoError(d.Insert())

	staticHosts, err := doStaticHostUpdate(d)
	assert.NoError(err)
	for _, h := range staticHosts {
		host, err := host.FindOne(host.ById(h))
		assert.NoError(err)
		assert.Equal(evergreen.ProviderNameDockerStatic, host.Provider)
		assert.True(host.HasContainers)
	}
}
