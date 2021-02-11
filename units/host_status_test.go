package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestCloudStatusJob(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(host.Collection))
	hosts := []host.Host{
		{
			Id:       "host-1",
			Provider: evergreen.ProviderNameMock,
			Status:   evergreen.HostStarting,
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameMock,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(birch.EC.String("region", "region-1"))},
			},
		},
		{
			Id:       "host-2",
			Provider: evergreen.ProviderNameMock,
			Status:   evergreen.HostStarting,
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameMock,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(birch.EC.String("region", "region-2"))},
			},
		},
		{
			Id:       "host-3",
			Provider: evergreen.ProviderNameMock,
			Status:   evergreen.HostTerminated,
		},
		{
			Id:       "host-4",
			Provider: evergreen.ProviderNameMock,
			Status:   evergreen.HostProvisioning,
		},
		{
			Id:       "host-5",
			Provider: evergreen.ProviderNameMock,
			Status:   evergreen.HostStarting,
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameMock,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(birch.EC.String("region", "region-3"))},
				BootstrapSettings:    distro.BootstrapSettings{Method: distro.BootstrapMethodNone},
			},
		},
		{
			Id:       "host-6",
			Provider: evergreen.ProviderNameMock,
			Status:   evergreen.HostStarting,
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameMock,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(birch.EC.String("region", "region-4"))},
				BootstrapSettings:    distro.BootstrapSettings{Method: distro.BootstrapMethodUserData},
			},
		},
	}
	mockState := cloud.GetMockProvider()
	mockState.Reset()
	for _, h := range hosts {
		require.NoError(h.Insert())
		mockState.Set(h.Id, cloud.MockInstance{DNSName: "dns_name"})
	}

	j := NewCloudHostReadyJob(&mock.Environment{}, "id")
	j.Run(context.Background())
	assert.NoError(j.Error())

	hosts, err := host.Find(db.Query(bson.M{}))
	assert.Len(hosts, 6)
	assert.NoError(err)
	for _, h := range hosts {
		if h.Id == "host-1" {
			assert.Equal(evergreen.HostProvisioning, h.Status)
		}
		if h.Id == "host-2" {
			assert.Equal(evergreen.HostProvisioning, h.Status)
		}
		if h.Id == "host-3" {
			assert.Equal(evergreen.HostTerminated, h.Status)
		}
		if h.Id == "host-4" {
			assert.Equal(evergreen.HostProvisioning, h.Status)
		}
		if h.Id == "host-5" {
			assert.Equal(evergreen.HostRunning, h.Status)
		}
		if h.Id == "host-6" {
			assert.Equal(evergreen.HostStarting, h.Status)
			assert.True(h.Provisioned)
		}
	}
}

func TestTerminateUnknownHosts(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection))
	h1 := host.Host{
		Id: "h1",
	}
	require.NoError(t, h1.Insert())
	h2 := host.Host{
		Id: "h2",
	}
	require.NoError(t, h2.Insert())
	env := &mock.Environment{}
	ctx := context.Background()
	require.NoError(t, env.Configure(ctx))
	j := NewCloudHostReadyJob(env, "id").(*cloudHostReadyJob)
	awsErr := "error getting host statuses for providers: error describing instances: after 10 retries, operation failed: InvalidInstanceID.NotFound: The instance IDs 'h1, h2' do not exist"
	assert.NoError(t, j.terminateUnknownHosts(ctx, awsErr))
}

func TestSetCloudHostStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, j *cloudHostReadyJob, mockMgr cloud.Manager){
		"RunningStatusPreparesLegacyHostForProvisioning": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, j *cloudHostReadyJob, mockMgr cloud.Manager) {
			provider := cloud.GetMockProvider()
			mockInstance := cloud.MockInstance{
				IsUp:    true,
				Status:  cloud.StatusRunning,
				DNSName: "dns_name",
			}
			provider.Set(h.Id, mockInstance)

			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodLegacySSH
			require.NoError(t, h.Insert())

			require.NoError(t, j.setCloudHostStatus(ctx, mockMgr, *h, mockInstance.Status))
			assert.True(t, provider.Get(h.Id).OnUpRan)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, mockInstance.DNSName, dbHost.Host)
			assert.False(t, dbHost.Provisioned)
			assert.Equal(t, evergreen.HostProvisioning, dbHost.Status)
		},
		"RunningStatusMarksUserDataProvisionedHostAsProvisioned": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, j *cloudHostReadyJob, mockMgr cloud.Manager) {
			provider := cloud.GetMockProvider()
			mockInstance := cloud.MockInstance{
				IsUp:    true,
				Status:  cloud.StatusRunning,
				DNSName: "dns_name",
			}
			provider.Set(h.Id, mockInstance)

			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodUserData
			require.NoError(t, h.Insert())

			require.NoError(t, j.setCloudHostStatus(ctx, mockMgr, *h, mockInstance.Status))
			assert.True(t, provider.Get(h.Id).OnUpRan)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, mockInstance.DNSName, dbHost.Host)
			assert.Equal(t, evergreen.HostStarting, dbHost.Status)
			assert.True(t, dbHost.Provisioned)
		},
		"FailedStatusInitiatesTermination": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, j *cloudHostReadyJob, mockMgr cloud.Manager) {
			require.NoError(t, h.Insert())
			require.NoError(t, j.setCloudHostStatus(ctx, mockMgr, *h, cloud.StatusFailed))

			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 100*time.Millisecond))

			assert.Equal(t, cloud.StatusTerminated, cloud.GetMockProvider().Get(h.Id).Status)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"StoppedStatusInitiatesTermination": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, j *cloudHostReadyJob, mockMgr cloud.Manager) {
			tsk := task.Task{
				Id: "some_task",
			}
			require.NoError(t, tsk.Insert())
			h.RunningTask = tsk.Id
			require.NoError(t, h.Insert())
			require.NoError(t, j.setCloudHostStatus(ctx, mockMgr, *h, cloud.StatusStopped))

			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 100*time.Millisecond))

			assert.Equal(t, cloud.StatusTerminated, cloud.GetMockProvider().Get(h.Id).Status)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
			assert.Zero(t, dbHost.RunningTask)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 5*time.Second)
			defer tcancel()
			require.NoError(t, db.ClearCollections(host.Collection, task.Collection))
			defer func() {
				assert.NoError(t, db.ClearCollections(host.Collection, task.Collection))
			}()

			env := &mock.Environment{}
			ctx := context.Background()
			require.NoError(t, env.Configure(ctx))

			h := &host.Host{
				Id:       "id",
				Provider: evergreen.ProviderNameMock,
				Distro: distro.Distro{
					Provider: evergreen.ProviderNameMock,
				},
				Status: evergreen.HostStarting,
			}

			j, ok := NewCloudHostReadyJob(env, h.Id).(*cloudHostReadyJob)
			require.True(t, ok)

			provider := cloud.GetMockProvider()
			provider.Reset()
			defer provider.Reset()

			mockMgr, err := cloud.GetManager(tctx, env, cloud.ManagerOpts{
				Provider: evergreen.ProviderNameMock,
			})
			require.NoError(t, err)
			_, err = mockMgr.SpawnHost(tctx, h)
			require.NoError(t, err)

			testCase(tctx, t, env, h, j, mockMgr)
		})
	}
}
