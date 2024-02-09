package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestDeleteDistroById(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(distro.Collection, event.EventCollection, user.Collection, model.TaskQueuesCollection, host.Collection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context, u user.DBUser){
		"Successfully deletes distro and clears task queue": func(t *testing.T, ctx context.Context, u user.DBUser) {
			assert.NoError(t, DeleteDistroById(ctx, &u, "distro"))

			dbDistro, err := distro.FindOneId(ctx, "distro")
			assert.NoError(t, err)
			assert.Nil(t, dbDistro)

			dbHost, err := host.FindOneId(ctx, "host")
			assert.NoError(t, err)
			assert.Equal(t, dbHost.Status, evergreen.HostTerminated)

			dbQueue, err := model.LoadTaskQueue("distro")
			assert.NoError(t, err)
			assert.Empty(t, dbQueue.Queue)

			events, err := event.FindLatestPrimaryDistroEvents("distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Equal(t, len(events), 1)
		},
		"Succeeds even if task queue for distro does not exist": func(t *testing.T, ctx context.Context, u user.DBUser) {
			err := DeleteDistroById(ctx, &u, "distro-no-task-queue")
			assert.NoError(t, err)

			events, err := event.FindLatestPrimaryDistroEvents("distro-no-task-queue", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Equal(t, len(events), 1)
		},
		"Fails when distro does not exist": func(t *testing.T, ctx context.Context, u user.DBUser) {
			err := DeleteDistroById(ctx, &u, "nonexistent")
			assert.Error(t, err)
			assert.Equal(t, err.Error(), "400 (Bad Request): distro 'nonexistent' not found")

			events, err := event.FindLatestPrimaryDistroEvents("nonexistent", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Equal(t, len(events), 0)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()
			assert.NoError(t, db.ClearCollections(distro.Collection, event.EventCollection, user.Collection, model.TaskQueuesCollection, host.Collection))

			d := distro.Distro{
				Id: "distro",
			}
			assert.NoError(t, d.Insert(tctx))

			host := host.Host{
				Id:     "host",
				Status: evergreen.HostRunning,
				Distro: distro.Distro{
					Id: d.Id,
				},
				Provider: evergreen.HostTypeStatic,
			}
			assert.NoError(t, host.Insert(tctx))

			queue := model.TaskQueue{
				Distro: d.Id,
				Queue:  []model.TaskQueueItem{{Id: "task"}},
			}
			assert.NoError(t, queue.Save())

			d.Id = "distro-no-task-queue"
			assert.NoError(t, d.Insert(tctx))

			adminUser := user.DBUser{
				Id: "admin",
			}
			assert.NoError(t, adminUser.Insert())

			tCase(t, tctx, adminUser)
		})
	}
}

func TestCopyDistro(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := evergreen.GetConfig(ctx)
	assert.NoError(t, err)
	config.Keys = map[string]string{"abc": "123"}
	assert.NoError(t, config.Set(ctx))
	defer func() {
		config.Keys = map[string]string{}
		assert.NoError(t, config.Set(ctx))
	}()

	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context, u user.DBUser){
		"Successfully copies distro": func(t *testing.T, ctx context.Context, u user.DBUser) {

			opts := CopyDistroOpts{
				DistroIdToCopy: "distro",
				NewDistroId:    "new-distro",
			}
			assert.NoError(t, CopyDistro(ctx, &u, opts))

			newDistro, err := distro.FindOneId(ctx, "new-distro")
			assert.NoError(t, err)
			assert.NotNil(t, newDistro)

			events, err := event.FindLatestPrimaryDistroEvents("new-distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Equal(t, len(events), 1)
		},
		"Fails when the validator encounters an error": func(t *testing.T, ctx context.Context, u user.DBUser) {
			opts := CopyDistroOpts{
				DistroIdToCopy: "distro",
				NewDistroId:    "distro2",
			}

			err := CopyDistro(ctx, &u, opts)
			assert.Error(t, err)
			assert.Equal(t, err.Error(), "validator encountered errors: 'ERROR: distro 'distro2' uses an existing identifier'")

			events, err := event.FindLatestPrimaryDistroEvents("distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Equal(t, len(events), 0)
		},
		"Fails with 400 when providing the same ID for original and output": func(t *testing.T, ctx context.Context, u user.DBUser) {
			opts := CopyDistroOpts{
				DistroIdToCopy: "distro",
				NewDistroId:    "distro",
			}
			err := CopyDistro(ctx, &u, opts)
			assert.Error(t, err)
			assert.Equal(t, err.Error(), "400 (Bad Request): new and existing distro IDs are identical")

			events, err := event.FindLatestPrimaryDistroEvents("distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Equal(t, len(events), 0)
		},
		"Fails when distro to copy does not exist": func(t *testing.T, ctx context.Context, u user.DBUser) {
			opts := CopyDistroOpts{
				DistroIdToCopy: "my-distro",
				NewDistroId:    "new-distro",
			}
			err := CopyDistro(ctx, &u, opts)
			assert.Error(t, err)
			assert.Equal(t, err.Error(), "404 (Not Found): distro 'my-distro' not found")

			events, err := event.FindLatestPrimaryDistroEvents("new-distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Equal(t, len(events), 0)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()

			assert.NoError(t, db.ClearCollections(distro.Collection, event.EventCollection, user.Collection))

			d := distro.Distro{
				Id:                 "distro",
				Arch:               "linux_amd64",
				AuthorizedKeysFile: "keys.txt",
				BootstrapSettings: distro.BootstrapSettings{
					Method: distro.BootstrapMethodNone,
				},
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
			assert.NoError(t, d.Insert(tctx))

			d.Id = "distro2"
			assert.NoError(t, d.Insert(tctx))

			adminUser := user.DBUser{
				Id: "admin",
			}
			assert.NoError(t, adminUser.Insert())

			tCase(t, tctx, adminUser)
		})
	}
}

func TestCreateDistro(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := evergreen.GetConfig(ctx)
	assert.NoError(t, err)
	config.Keys = map[string]string{"abc": "123"}
	assert.NoError(t, config.Set(ctx))
	defer func() {
		config.Keys = map[string]string{}
		assert.NoError(t, config.Set(ctx))
	}()

	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context, u user.DBUser){
		"Successfully creates distro": func(t *testing.T, ctx context.Context, u user.DBUser) {
			assert.NoError(t, CreateDistro(ctx, &u, "new-distro"))

			newDistro, err := distro.FindOneId(ctx, "new-distro")
			assert.NoError(t, err)
			assert.NotNil(t, newDistro)

			events, err := event.FindLatestPrimaryDistroEvents("new-distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Equal(t, len(events), 1)
		},
		"Fails when the validator encounters an error": func(t *testing.T, ctx context.Context, u user.DBUser) {
			err := CreateDistro(ctx, &u, "distro")
			assert.Error(t, err)
			assert.Equal(t, err.Error(), "validator encountered errors: 'ERROR: distro 'distro' uses an existing identifier'")

			events, err := event.FindLatestPrimaryDistroEvents("distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Equal(t, len(events), 0)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()

			assert.NoError(t, db.ClearCollections(distro.Collection, event.EventCollection, user.Collection))

			adminUser := user.DBUser{
				Id: "admin",
			}
			assert.NoError(t, adminUser.Insert())

			d := distro.Distro{
				Id:                 "distro",
				Arch:               "linux_amd64",
				AuthorizedKeysFile: "keys.txt",
				BootstrapSettings: distro.BootstrapSettings{
					Method: distro.BootstrapMethodNone,
				},
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
			assert.NoError(t, d.Insert(tctx))

			tCase(t, tctx, adminUser)
		})
	}
}
