package scheduler

import (
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeedsReprovision(t *testing.T) {
	for testName, testCase := range map[string]struct {
		d        distro.Distro
		h        *host.Host
		expected host.ReprovisionType
	}{
		"NewLegacyHostDoesNotNeedReprovisioning": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodSSH,
				},
			},
			h:        nil,
			expected: host.ReprovisionNone,
		},
		"NewNonLegacyHostNeedsNewProvisioning": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.CommunicationMethodSSH,
				},
			},
			h:        nil,
			expected: host.ReprovisionToNew,
		},
		"ExistingLegacyHostWithNonLegacyDistroBootstrappingNeedsNewProvisioning": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.CommunicationMethodSSH,
				},
			},
			h: &host.Host{
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodLegacySSH,
						Communication: distro.CommunicationMethodLegacySSH,
					},
				},
			},
			expected: host.ReprovisionToNew,
		},
		"ExistingHostWithLegacyDistroBootstrappingNeedsLegacyProvisioning": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
			},
			h: &host.Host{
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
				},
			},
			expected: host.ReprovisionToLegacy,
		},
		"ExistingNonLegacyHostPreservesExistingReprovisioning": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.CommunicationMethodSSH,
				},
			},
			h: &host.Host{
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
				},
				NeedsReprovision: host.ReprovisionToNew,
			},
			expected: host.ReprovisionToNew,
		},
		"ExistingNonLegacyHostStillNeedsToRestartJasper": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.CommunicationMethodSSH,
				},
			},
			h: &host.Host{
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
				},
				NeedsReprovision: host.ReprovisionRestartJasper,
			},
			expected: host.ReprovisionRestartJasper,
		},
		"ExistingNonLegacyHostTransitioningToLegacyDoesNotNeedToRestartJasper": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
			},
			h: &host.Host{
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
				},
				NeedsReprovision: host.ReprovisionRestartJasper,
			},
			expected: host.ReprovisionNone,
		},
		"ExistingLegacyHostPreservesExistingLegacyReprovisioning": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
			},
			h: &host.Host{
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodLegacySSH,
						Communication: distro.CommunicationMethodLegacySSH,
					},
				},
				NeedsReprovision: host.ReprovisionToLegacy,
			},
			expected: host.ReprovisionToLegacy,
		},
		"ExistingLegacyHostDoesNotNeedToRestartJasper": {
			d: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
			},
			h: &host.Host{
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodLegacySSH,
						Communication: distro.CommunicationMethodLegacySSH,
					},
				},
				NeedsReprovision: host.ReprovisionRestartJasper,
			},
			expected: host.ReprovisionNone,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			assert.Equal(t, testCase.expected, needsReprovisioning(testCase.d, testCase.h))
		})
	}
}

func makeStaticHostProviderSettings(t *testing.T, names ...string) *birch.Document {
	settings := cloud.StaticSettings{
		Hosts: []cloud.StaticHost{},
	}
	for _, name := range names {
		settings.Hosts = append(settings.Hosts, cloud.StaticHost{Name: name})
	}
	b, err := bson.Marshal(settings)
	require.NoError(t, err)
	doc := &birch.Document{}
	require.NoError(t, doc.UnmarshalBSON(b))
	return doc
}

func TestDoStaticHostUpdate(t *testing.T) {
	legacyHost := func() *host.Host {
		return &host.Host{
			Id:   "host1",
			User: "user1",
			Distro: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
			},
			Status:      evergreen.HostRunning,
			Provisioned: true,
		}
	}
	nonLegacyHost := func() *host.Host {
		return &host.Host{
			Id:   "host2",
			User: "user1",
			Distro: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.CommunicationMethodSSH,
				},
			},
			Status:      evergreen.HostRunning,
			Provisioned: true,
		}
	}

	for testName, testCase := range map[string]func(t *testing.T){
		"NewHostWithoutUserUsesDistroUser": func(t *testing.T) {
			name := "host"
			user := "user"
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, name)},
				Provider:             evergreen.ProviderNameStatic,
				User:                 user,
			}

			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, name, hosts[0])

			dbHost, err := host.FindOneId(name)
			require.NoError(t, err)
			assert.Equal(t, user, dbHost.User)
			assert.Equal(t, evergreen.User, dbHost.StartedBy)
			assert.Equal(t, name, dbHost.Host)
			assert.Equal(t, evergreen.HostTypeStatic, dbHost.Provider)
			assert.WithinDuration(t, time.Now(), dbHost.CreationTime, time.Minute)
			assert.Equal(t, d.Id, dbHost.Distro.Id)
			assert.True(t, dbHost.Provisioned)
		},
		"NewHostOnLegacyDistro": func(t *testing.T) {
			name := "user@host"
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, name)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
			}

			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, name, hosts[0])

			dbHost, err := host.FindOneId(name)
			require.NoError(t, err)
			assert.Equal(t, dbHost.Status, evergreen.HostRunning)
			assert.Equal(t, host.ReprovisionNone, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"NewHostOnNonLegacyDistro": func(t *testing.T) {
			name := "user@host"
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, name)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.BootstrapMethodSSH,
				},
			}

			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, name, hosts[0])

			dbHost, err := host.FindOneId(name)
			require.NoError(t, err)
			assert.Equal(t, dbHost.Status, evergreen.HostProvisioning)
			assert.Equal(t, host.ReprovisionToNew, dbHost.NeedsReprovision)
			assert.False(t, dbHost.Provisioned)
		},
		"LegacyHostOnLegacyDistro": func(t *testing.T) {
			h := legacyHost()
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.BootstrapMethodLegacySSH,
				},
			}
			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
			assert.Equal(t, host.ReprovisionNone, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"LegacyHostOnNonLegacyDistro": func(t *testing.T) {
			h := legacyHost()
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.BootstrapMethodSSH,
				},
			}
			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
			assert.Equal(t, host.ReprovisionToNew, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"NonLegacyHostOnLegacyDistro": func(t *testing.T) {
			h := nonLegacyHost()
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.BootstrapMethodLegacySSH,
				},
			}
			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
			assert.Equal(t, host.ReprovisionToLegacy, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"NonLegacyHostOnNonLegacyDistro": func(t *testing.T) {
			h := nonLegacyHost()
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.BootstrapMethodSSH,
				},
			}

			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
			assert.Equal(t, host.ReprovisionNone, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"TerminatedLegacyHostOnLegacyDistro": func(t *testing.T) {
			h := legacyHost()
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.BootstrapMethodLegacySSH,
				},
			}

			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
			assert.Equal(t, host.ReprovisionNone, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"TerminatedLegacyHostOnNonLegacyDistro": func(t *testing.T) {
			h := legacyHost()
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.BootstrapMethodSSH,
				},
			}

			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
			assert.Equal(t, host.ReprovisionToNew, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"TerminatedNonLegacyHostOnNonLegacyDistro": func(t *testing.T) {
			h := nonLegacyHost()
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.BootstrapMethodSSH,
				},
			}

			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
			assert.Equal(t, host.ReprovisionNone, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"TerminatedNonLegacyHostOnLegacyDistro": func(t *testing.T) {
			h := nonLegacyHost()
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.BootstrapMethodLegacySSH,
				},
			}

			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
			assert.Equal(t, host.ReprovisionToLegacy, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"QuarantinedLegacyHostOnLegacyDistro": func(t *testing.T) {
			h := legacyHost()
			h.Status = evergreen.HostQuarantined
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.BootstrapMethodLegacySSH,
				},
			}

			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostQuarantined, dbHost.Status)
			assert.Equal(t, host.ReprovisionNone, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"QuarantinedLegacyHostOnNonLegacyDistro": func(t *testing.T) {
			h := legacyHost()
			h.Status = evergreen.HostQuarantined
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.BootstrapMethodSSH,
				},
			}

			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostQuarantined, dbHost.Status)
			assert.Equal(t, host.ReprovisionToNew, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"QuarantinedNonLegacyHostOnNonLegacyDistro": func(t *testing.T) {
			h := nonLegacyHost()
			h.Status = evergreen.HostQuarantined
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.BootstrapMethodSSH,
				},
			}

			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostQuarantined, dbHost.Status)
			assert.Equal(t, host.ReprovisionNone, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"QuarantinedNonLegacyHostOnLegacyDistro": func(t *testing.T) {
			h := nonLegacyHost()
			h.Status = evergreen.HostQuarantined
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.BootstrapMethodLegacySSH,
				},
			}

			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostQuarantined, dbHost.Status)
			assert.Equal(t, host.ReprovisionToLegacy, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"ProvisioningLegacyHostOnLegacyDistro": func(t *testing.T) {
			h := legacyHost()
			h.Status = evergreen.HostProvisioning
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.BootstrapMethodLegacySSH,
				},
			}
			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
			assert.Equal(t, host.ReprovisionNone, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
		"ProvisioningNonLegacyHostOnNonLegacyDistro": func(t *testing.T) {
			h := nonLegacyHost()
			h.Status = evergreen.HostProvisioning
			require.NoError(t, h.Insert())
			d := distro.Distro{
				Id:                   "distro",
				ProviderSettingsList: []*birch.Document{makeStaticHostProviderSettings(t, h.Id)},
				Provider:             evergreen.ProviderNameStatic,
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.BootstrapMethodSSH,
				},
			}
			hosts, err := doStaticHostUpdate(d)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0])

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
			assert.Equal(t, host.ReprovisionNone, dbHost.NeedsReprovision)
			assert.True(t, dbHost.Provisioned)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(host.Collection))
			defer func() {
				assert.NoError(t, db.Clear(host.Collection))
			}()
			testCase(t)
		})
	}
}
