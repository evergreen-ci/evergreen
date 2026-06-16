package host

import (
	"context"
	"sort"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertTestDocuments(ctx context.Context) error {
	input := []any{
		Host{
			Id:     "one",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "debian",
				Provider: evergreen.ProviderNameEc2Fleet,
			},
			RunningTask: "baz",
			StartedBy:   evergreen.User,
		},
		Host{
			Id:     "two",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "redhat",
				Provider: evergreen.ProviderNameEc2Fleet,
			},
			RunningTask: "bar",
			StartedBy:   evergreen.User,
		},
		Host{
			Id:     "three",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "debian",
				Provider: evergreen.ProviderNameEc2Fleet,
			},
			RunningTask: "foo-foo",
			StartedBy:   evergreen.User,
		},
		Host{
			Id:     "four",
			Status: evergreen.HostUninitialized,
			Distro: distro.Distro{
				Id:       "foo",
				Provider: evergreen.ProviderNameEc2Fleet,
			},
			StartedBy: evergreen.User,
		},
		Host{
			Id:     "five",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "bar",
				Provider: evergreen.ProviderNameStatic,
			},
			StartedBy: evergreen.User,
		},
		Host{
			Id:     "six",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "debian",
				Provider: evergreen.ProviderNameStatic,
			},
			RunningTask: "foo",
		},
		Host{
			Id:     "seven",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:               "release",
				Provider:         evergreen.ProviderNameStatic,
				SingleTaskDistro: true,
			},
			StartedBy: evergreen.User,
		},
	}

	return db.InsertMany(ctx, Collection, input...)
}

func TestHostStatsByProvider(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	defer func() {
		assert.NoError(db.ClearCollections(Collection))
	}()
	assert.NoError(insertTestDocuments(t.Context()))

	result := ProviderStats{}

	assert.NoError(db.Aggregate(t.Context(), Collection, statsByProviderPipeline(), &result))
	assert.Len(result, 2, "%+v", result)

	rmap := result.Map()
	assert.Equal(3, rmap[evergreen.ProviderNameEc2Fleet])

	alt, err := GetProviderCounts(t.Context())
	assert.NoError(err)
	sort.Slice(alt, func(i, j int) bool { return alt[i].Provider < alt[j].Provider })
	sort.Slice(result, func(i, j int) bool { return result[i].Provider < result[j].Provider })
	assert.Equal(alt, result)
}

func TestHostStatsByDistro(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	defer func() {
		assert.NoError(db.ClearCollections(Collection))
	}()
	assert.NoError(insertTestDocuments(t.Context()))

	result := DistroStats{}

	assert.NoError(db.Aggregate(t.Context(), Collection, statsByDistroPipeline(), &result))
	assert.Len(result, 4, "%+v", result)

	rcmap := result.CountMap()
	assert.Equal(2, rcmap["debian"])
	assert.Equal(1, rcmap["redhat"])

	rtmap := result.TasksMap()
	assert.Equal(2, rtmap["debian"])
	assert.Equal(1, rtmap["redhat"])

	exceeded := result.MaxHostsExceeded()
	assert.Len(exceeded, 2)
	assert.NotContains(exceeded, "bar")

	for _, stat := range result {
		if stat.Distro == "release" {
			assert.True(stat.SingleTaskDistro)
		} else {
			assert.False(stat.SingleTaskDistro)
		}
	}

	alt, err := GetStatsByDistro(t.Context())
	assert.NoError(err)
	sort.Slice(alt, func(i, j int) bool { return alt[i].Distro < alt[j].Distro })
	sort.Slice(result, func(i, j int) bool { return result[i].Distro < result[j].Distro })
	assert.Equal(alt, result)
}

func TestAggregateSpawnhostCountByProject(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, task.Collection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(Collection, task.Collection))
	})

	t.Run("MultipleProjectsReturnsSortedByCountDescending", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection, task.Collection))

		for _, tk := range []task.Task{
			{Id: "task1", Project: "projectA"},
			{Id: "task2", Project: "projectA"},
			{Id: "task3", Project: "projectB"},
		} {
			require.NoError(t, tk.Insert(t.Context()))
		}

		for _, h := range []Host{
			{Id: "h1", UserHost: true, Status: evergreen.HostRunning, ProvisionOptions: &ProvisionOptions{TaskId: "task1"}},
			{Id: "h2", UserHost: true, Status: evergreen.HostRunning, ProvisionOptions: &ProvisionOptions{TaskId: "task2"}},
			{Id: "h3", UserHost: true, Status: evergreen.HostStarting, ProvisionOptions: &ProvisionOptions{TaskId: "task3"}},
		} {
			require.NoError(t, h.Insert(t.Context()))
		}

		results, err := AggregateSpawnhostCountByProject(t.Context())
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, "projectA", results[0].Project)
		assert.Equal(t, 2, results[0].Count)
		assert.Equal(t, "projectB", results[1].Project)
		assert.Equal(t, 1, results[1].Count)
	})

	t.Run("ExcludesNonSpawnHostsAndTerminatedHosts", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection, task.Collection))

		require.NoError(t, (&task.Task{Id: "task1", Project: "projectA"}).Insert(t.Context()))
		require.NoError(t, (&task.Task{Id: "task2", Project: "projectB"}).Insert(t.Context()))
		require.NoError(t, (&task.Task{Id: "task3", Project: "projectC"}).Insert(t.Context()))

		for _, h := range []Host{
			{Id: "h1", UserHost: false, Status: evergreen.HostRunning, ProvisionOptions: &ProvisionOptions{TaskId: "task1"}},
			{Id: "h2", UserHost: true, Status: evergreen.HostTerminated, ProvisionOptions: &ProvisionOptions{TaskId: "task2"}},
			{Id: "h3", UserHost: true, Status: evergreen.HostRunning, ProvisionOptions: &ProvisionOptions{TaskId: ""}},
			{Id: "h4", UserHost: true, Status: evergreen.HostRunning, ProvisionOptions: &ProvisionOptions{TaskId: "nonexistent_task"}},
			{Id: "h5", UserHost: true, Status: evergreen.HostRunning, ProvisionOptions: &ProvisionOptions{TaskId: "task3"}},
		} {
			require.NoError(t, h.Insert(t.Context()))
		}

		results, err := AggregateSpawnhostCountByProject(t.Context())
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "projectC", results[0].Project)
		assert.Equal(t, 1, results[0].Count)
	})
}
