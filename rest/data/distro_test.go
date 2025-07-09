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
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)

			dbQueue, err := model.LoadTaskQueue(t.Context(), "distro")
			assert.NoError(t, err)
			assert.Empty(t, dbQueue.Queue)

			events, err := event.FindLatestPrimaryDistroEvents(t.Context(), "distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Len(t, events, 1)
		},
		"Succeeds even if task queue for distro does not exist": func(t *testing.T, ctx context.Context, u user.DBUser) {
			err := DeleteDistroById(ctx, &u, "distro-no-task-queue")
			assert.NoError(t, err)

			events, err := event.FindLatestPrimaryDistroEvents(t.Context(), "distro-no-task-queue", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Len(t, events, 1)
		},
		"Fails when distro does not exist": func(t *testing.T, ctx context.Context, u user.DBUser) {
			err := DeleteDistroById(ctx, &u, "nonexistent")
			assert.Error(t, err)
			assert.Equal(t, "400 (Bad Request): distro 'nonexistent' not found", err.Error())

			events, err := event.FindLatestPrimaryDistroEvents(t.Context(), "nonexistent", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Empty(t, events)
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
			assert.NoError(t, queue.Save(t.Context()))

			d.Id = "distro-no-task-queue"
			assert.NoError(t, d.Insert(tctx))

			adminUser := user.DBUser{
				Id: "admin",
			}
			assert.NoError(t, adminUser.Insert(t.Context()))

			tCase(t, tctx, adminUser)
		})
	}
}

func TestCopyDistro(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context, u user.DBUser){
		"Successfully copies distro": func(t *testing.T, ctx context.Context, u user.DBUser) {
			opts := restModel.CopyDistroOpts{
				DistroIdToCopy: "distro",
				NewDistroId:    "new-distro",
			}
			assert.NoError(t, CopyDistro(ctx, &u, opts))

			newDistro, err := distro.FindOneId(ctx, "new-distro")
			assert.NoError(t, err)
			require.NotNil(t, newDistro)
			assert.Nil(t, newDistro.Aliases)

			events, err := event.FindLatestPrimaryDistroEvents(t.Context(), "new-distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Len(t, events, 1)
		},
		"Fails when the validator encounters an error": func(t *testing.T, ctx context.Context, u user.DBUser) {
			opts := restModel.CopyDistroOpts{
				DistroIdToCopy: "distro",
				NewDistroId:    "distro2",
			}

			err := CopyDistro(ctx, &u, opts)
			assert.Error(t, err)
			assert.Equal(t, "validator encountered errors: 'ERROR: distro 'distro2' uses an existing identifier'", err.Error())

			events, err := event.FindLatestPrimaryDistroEvents(t.Context(), "distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Empty(t, events)
		},
		"Fails with 400 when providing the same ID for original and output": func(t *testing.T, ctx context.Context, u user.DBUser) {
			opts := restModel.CopyDistroOpts{
				DistroIdToCopy: "distro",
				NewDistroId:    "distro",
			}
			err := CopyDistro(ctx, &u, opts)
			assert.Error(t, err)
			assert.Equal(t, "400 (Bad Request): new and existing distro IDs are identical", err.Error())

			events, err := event.FindLatestPrimaryDistroEvents(t.Context(), "distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Empty(t, events)
		},
		"Fails when distro to copy does not exist": func(t *testing.T, ctx context.Context, u user.DBUser) {
			opts := restModel.CopyDistroOpts{
				DistroIdToCopy: "my-distro",
				NewDistroId:    "new-distro",
			}
			err := CopyDistro(ctx, &u, opts)
			assert.Error(t, err)
			assert.Equal(t, "404 (Not Found): distro 'my-distro' not found", err.Error())

			events, err := event.FindLatestPrimaryDistroEvents(t.Context(), "new-distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Empty(t, events)
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
					Version: evergreen.DispatcherVersionRevisedWithDependencies,
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
				WorkDir:  "/tmp",
				User:     "admin",
				Aliases:  []string{"alias1", "alias2"},
			}
			assert.NoError(t, d.Insert(tctx))

			d.Id = "distro2"
			assert.NoError(t, d.Insert(tctx))

			adminUser := user.DBUser{
				Id: "admin",
			}
			assert.NoError(t, adminUser.Insert(t.Context()))

			tCase(t, tctx, adminUser)
		})
	}
}

func TestUpdateDistro(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context, old, new *distro.Distro){
		"Successfully updates distro": func(t *testing.T, ctx context.Context, old, new *distro.Distro) {
			assert.NoError(t, UpdateDistro(ctx, old, new))

			dbDistro, err := distro.FindOneId(ctx, old.Id)
			assert.NoError(t, err)
			assert.NotNil(t, dbDistro)
			assert.Equal(t, *dbDistro, *new)

			dbQueue, err := model.LoadTaskQueue(t.Context(), "distro")
			assert.NoError(t, err)
			assert.NotEmpty(t, dbQueue.Queue)
		},
		"Clears task queue when distro gets disabled": func(t *testing.T, ctx context.Context, old, new *distro.Distro) {
			new.Disabled = true
			assert.NoError(t, UpdateDistro(ctx, old, new))

			dbDistro, err := distro.FindOneId(ctx, old.Id)
			assert.NoError(t, err)
			assert.NotNil(t, dbDistro)
			assert.Equal(t, *dbDistro, *new)

			dbQueue, err := model.LoadTaskQueue(t.Context(), "distro")
			assert.NoError(t, err)
			assert.Empty(t, dbQueue.Queue)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()

			assert.NoError(t, db.ClearCollections(distro.Collection, event.EventCollection, user.Collection, model.TaskQueuesCollection))

			d := distro.Distro{
				Id: "distro",
				PlannerSettings: distro.PlannerSettings{
					Version: evergreen.PlannerVersionTunable,
				},
				Provider: evergreen.ProviderNameStatic,
				WorkDir:  "/tmp",
				User:     "admin",
			}
			assert.NoError(t, d.Insert(tctx))

			updatedDistro := distro.Distro{
				Id: "distro",
				PlannerSettings: distro.PlannerSettings{
					Version: evergreen.PlannerVersionTunable,
				},
				Provider: evergreen.ProviderNameEc2OnDemand,
				WorkDir:  "/tmp2",
				User:     "admin2",
			}

			queue := model.TaskQueue{
				Distro: d.Id,
				Queue:  []model.TaskQueueItem{{Id: "task"}},
			}
			assert.NoError(t, queue.Save(t.Context()))

			tCase(t, tctx, &d, &updatedDistro)
		})
	}
}

func TestCreateDistro(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, u user.DBUser){
		"SuccessfullyCreatesDistro": func(t *testing.T, u user.DBUser) {
			assert.NoError(t, CreateDistro(t.Context(), &u, "new-distro", false))

			newDistro, err := distro.FindOneId(t.Context(), "new-distro")
			assert.NoError(t, err)
			require.NotNil(t, newDistro)

			events, err := event.FindLatestPrimaryDistroEvents(t.Context(), "new-distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Len(t, events, 1)

			assert.False(t, newDistro.SingleTaskDistro, "expected distro to not be a single task distro")
		},
		"SuccessfullyCreatesSingleTaskDistro": func(t *testing.T, u user.DBUser) {
			assert.NoError(t, CreateDistro(t.Context(), &u, "new-distro", true))

			newDistro, err := distro.FindOneId(t.Context(), "new-distro")
			assert.NoError(t, err)
			assert.NotNil(t, newDistro)

			events, err := event.FindLatestPrimaryDistroEvents(t.Context(), "new-distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Len(t, events, 1)

			assert.True(t, newDistro.SingleTaskDistro, "expected distro to be a single task distro")
		},
		"FailsWhenTheValidatorEncountersAnError": func(t *testing.T, u user.DBUser) {
			err := CreateDistro(t.Context(), &u, "distro", false)
			assert.Error(t, err)
			assert.Equal(t, "validator encountered errors: 'ERROR: distro 'distro' uses an existing identifier'", err.Error())

			events, err := event.FindLatestPrimaryDistroEvents(t.Context(), "distro", 10, utility.ZeroTime)
			assert.NoError(t, err)
			assert.Empty(t, events)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(distro.Collection, event.EventCollection, user.Collection))

			adminUser := user.DBUser{
				Id: "admin",
			}
			assert.NoError(t, adminUser.Insert(t.Context()))

			d := distro.Distro{
				Id:                 "distro",
				Arch:               "linux_amd64",
				AuthorizedKeysFile: "keys.txt",
				BootstrapSettings: distro.BootstrapSettings{
					Method: distro.BootstrapMethodNone,
				},
				DispatcherSettings: distro.DispatcherSettings{
					Version: evergreen.DispatcherVersionRevisedWithDependencies,
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
				WorkDir:  "/tmp",
				User:     "admin",
			}
			assert.NoError(t, d.Insert(t.Context()))

			tCase(t, adminUser)
		})
	}
}
