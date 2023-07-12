package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteDistroById(t *testing.T) {
	session, _, err := db.GetGlobalSessionFactory().GetSession()
	require.NoError(t, err)
	defer session.Close()
	require.NoError(t, session.DB(testConfig.Database.DB).DropDatabase())
	defer func() {
		assert.NoError(t, session.DB(testConfig.Database.DB).DropDatabase())
	}()

	d := distro.Distro{
		Id: "distro",
	}
	require.NoError(t, d.Insert())

	queue := model.TaskQueue{
		Distro: d.Id,
		Queue:  []model.TaskQueueItem{{Id: "task"}},
	}
	require.NoError(t, queue.Save())

	require.NoError(t, DeleteDistroById(d.Id))

	dbDistro, err := distro.FindOneId(d.Id)
	assert.NoError(t, err)
	assert.Nil(t, dbDistro)

	dbQueue, err := model.LoadTaskQueue(queue.Distro)
	require.NoError(t, err)
	assert.Empty(t, dbQueue.Queue)
}

func TestCopyDistro(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "oldAdmin"})

	config, err := evergreen.GetConfig()
	assert.NoError(t, err)
	config.Keys = map[string]string{"abc": "123"}
	assert.NoError(t, config.Set())

	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context, env *mock.Environment, u user.DBUser){
		"Successfully copies distro": func(t *testing.T, ctx context.Context, env *mock.Environment, u user.DBUser) {

			opts := CopyDistroOpts{
				DistroIdToCopy: "distro",
				NewDistroId:    "new-distro",
			}
			assert.NoError(t, CopyDistro(ctx, env, &u, opts))

			newDistro, err := distro.FindOneId("new-distro")
			assert.NoError(t, err)
			assert.NotNil(t, newDistro)

			events, err := event.FindLatestPrimaryDistroEvents("new-distro", 10)
			assert.NoError(t, err)
			require.Equal(t, len(events), 1)
		},
		"Fails when the validator encounters an error": func(t *testing.T, ctx context.Context, env *mock.Environment, u user.DBUser) {

			opts := CopyDistroOpts{
				DistroIdToCopy: "distro",
				NewDistroId:    "distro",
			}
			err := CopyDistro(ctx, env, &u, opts)
			assert.Error(t, err)
			assert.Equal(t, err.Error(), "validator encountered errors: 'ERROR: distro 'distro' uses an existing identifier'")

			events, err := event.FindLatestPrimaryDistroEvents("distro", 10)
			assert.NoError(t, err)
			require.Equal(t, len(events), 0)
		},
		"Fails when distro to copy does not exist": func(t *testing.T, ctx context.Context, env *mock.Environment, u user.DBUser) {

			opts := CopyDistroOpts{
				DistroIdToCopy: "my-distro",
				NewDistroId:    "new-distro",
			}
			err := CopyDistro(ctx, env, &u, opts)
			assert.Error(t, err)
			assert.Equal(t, err.Error(), "404 (Not Found): distro 'my-distro' not found")

			events, err := event.FindLatestPrimaryDistroEvents("new-distro", 10)
			assert.NoError(t, err)
			require.Equal(t, len(events), 0)
		},
	} {
		t.Run(tName, func(t *testing.T) {

			tctx, tcancel := context.WithCancel(context.Background())
			defer tcancel()

			assert.NoError(t, db.ClearCollections(distro.Collection, event.EventCollection, user.Collection))

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			d := distro.Distro{
				Id:                 "distro",
				Arch:               "linux_amd64",
				AuthorizedKeysFile: "keys.txt",
				BootstrapSettings: distro.BootstrapSettings{
					Method: distro.BootstrapMethodNone,
				},
				CloneMethod: evergreen.CloneMethodLegacySSH,
				DispatcherSettings: distro.DispatcherSettings{
					Version: evergreen.DispatcherVersionRevised,
				},
				FinderSettings: distro.FinderSettings{
					Version: evergreen.FinderVersionParallel,
				},
				HostAllocatorSettings: distro.HostAllocatorSettings{
					Version: evergreen.HostAllocatorUtilization,
				},
				PlannerSettings: distro.PlannerSettings{
					Version: evergreen.PlannerVersionTunable,
				},
				Provider: evergreen.ProviderNameStatic,
				SSHKey:   "abc",
				WorkDir:  "/tmp",
				User:     "admin",
			}
			assert.NoError(t, d.Insert())

			adminUser := user.DBUser{
				Id: "admin",
			}
			require.NoError(t, adminUser.Insert())

			tCase(t, tctx, env, adminUser)
		})
	}
}
