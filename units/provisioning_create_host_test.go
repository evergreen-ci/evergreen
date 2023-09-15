package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	jmock "github.com/mongodb/jasper/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvisioningCreateHostJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, mgr *jmock.Manager, h *host.Host){
		"PopulatesFields": func(ctx context.Context, t *testing.T, env *mock.Environment, mgr *jmock.Manager, h *host.Host) {
			j := NewHostCreateJob(env, *h, "job-id", 0, true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)

			assert.Equal(t, env, hostCreateJob.env)
			assert.Equal(t, h.Id, hostCreateJob.HostID)
			assert.Equal(t, *h, *hostCreateJob.host)
		},
		"SucceedsForAMICreateHost": func(ctx context.Context, t *testing.T, env *mock.Environment, mgr *jmock.Manager, h *host.Host) {
			require.NoError(t, h.Insert(ctx))

			c := apimodels.CreateHost{
				AMI:                 "ami-654321",
				CloudProvider:       "ec2",
				NumHosts:            "1",
				Scope:               "task",
				SetupTimeoutSecs:    600,
				TeardownTimeoutSecs: 21600,
				AWSKeyID:            "my_aws_key",
				AWSSecret:           "my_secret_key",
				InstanceType:        "t1.micro",
				Subnet:              "subnet-123456",
				SecurityGroups:      []string{"1234"},
			}
			d, err := distro.GetHostCreateDistro(ctx, c)
			require.NoError(t, err)
			h.Distro = *d

			j := NewHostCreateJob(env, *h, "job-id", 0, true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)
			hostCreateJob.Run(ctx)
			assert.False(t, hostCreateJob.HasErrors())
			foundHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostBuilding, foundHost.Status)
		},
		"SucceedsForDistroCreateHost": func(ctx context.Context, t *testing.T, env *mock.Environment, mgr *jmock.Manager, h *host.Host) {
			require.NoError(t, h.Insert(ctx))

			c := apimodels.CreateHost{
				Distro:              "distro-id",
				CloudProvider:       "ec2",
				NumHosts:            "1",
				Scope:               "task",
				SetupTimeoutSecs:    600,
				TeardownTimeoutSecs: 21600,
				KeyName:             "mock_key",
			}
			d, err := distro.GetHostCreateDistro(ctx, c)
			require.NoError(t, err)
			h.Distro = *d

			j := NewHostCreateJob(env, *h, "job-id", 0, true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)
			hostCreateJob.Run(ctx)
			assert.False(t, hostCreateJob.HasErrors())
			foundHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostBuilding, foundHost.Status)
		},
		"NoopsForTerminatedHost": func(ctx context.Context, t *testing.T, env *mock.Environment, mgr *jmock.Manager, h *host.Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))

			j := NewHostCreateJob(env, *h, "job-id", 0, true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)
			hostCreateJob.Run(ctx)

			assert.False(t, hostCreateJob.HasErrors())
			foundHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostTerminated, foundHost.Status)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, distro.Collection))
			tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			tctx = testutil.TestSpan(tctx, t)

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))
			mgr := &jmock.Manager{}

			h := host.Host{
				Id:     "id",
				Status: evergreen.HostUninitialized,
				Distro: distro.Distro{
					Id: "distro-id",
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.BootstrapMethodSSH,
					},
					Arch: evergreen.ArchLinuxAmd64,
				},
				Host:     "localhost",
				User:     evergreen.User,
				ParentID: "parent-id",
			}
			d := distro.Distro{
				Id:       "distro-id",
				Provider: evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(
					birch.EC.String("region", evergreen.DefaultEC2Region),
					birch.EC.String("aws_access_key_id", "key"),
					birch.EC.String("aws_secret_access_key", "secret"),
				)},
			}
			require.NoError(t, d.Insert(tctx))
			parentHost := host.Host{
				Id:            "parent-id",
				HasContainers: true,
			}
			require.NoError(t, parentHost.Insert(tctx))

			testCase(tctx, t, env, mgr, &h)
		})
	}
}
