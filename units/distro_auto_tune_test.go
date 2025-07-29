package units

import (
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/hoststat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistroAutoTuneJob(t *testing.T) {
	colls := []string{distro.Collection, hoststat.Collection, event.EventCollection}
	defer func() {
		assert.NoError(t, db.ClearCollections(colls...))
	}()

	addDistroHostStats := func(t *testing.T, distroID string, numHosts ...int) {
		for _, num := range numHosts {
			stat := hoststat.NewHostStat(distroID, num)
			numMinsPerDay := 60 * 24
			stat.Timestamp = time.Now().Add(time.Duration(rand.IntN(numMinsPerDay)) * time.Minute)
			require.NoError(t, stat.Insert(t.Context()))
		}
	}

	for tName, tCase := range map[string]func(t *testing.T, env *mock.Environment, j *distroAutoTuneJob){
		"IncreasesMaxHostsWhenAlwaysHittingMaxHosts": func(t *testing.T, env *mock.Environment, j *distroAutoTuneJob) {
			originalMaxHosts := j.distro.HostAllocatorSettings.MaximumHosts
			require.NoError(t, j.distro.Insert(t.Context()))

			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{j.distro.HostAllocatorSettings.MaximumHosts}, 10)...)
			j.Run(t.Context())
			require.NoError(t, j.Error())

			dbDistro, err := distro.FindOneId(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDistro)
			assert.Greater(t, dbDistro.HostAllocatorSettings.MaximumHosts, originalMaxHosts, "max hosts should be increased due to always hitting max hosts")

			events, err := event.FindAllByResourceID(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.Len(t, events, 1)
			assert.Equal(t, event.ResourceTypeDistro, events[0].ResourceType)
			assert.Equal(t, event.EventDistroModified, events[0].EventType)
		},
		"IncreasesMaxHostsWhenHittingMaxHostsOccasionally": func(t *testing.T, env *mock.Environment, j *distroAutoTuneJob) {
			originalMaxHosts := j.distro.HostAllocatorSettings.MaximumHosts
			require.NoError(t, j.distro.Insert(t.Context()))

			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{j.distro.HostAllocatorSettings.MaximumHosts}, 10)...)
			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{0}, 20)...)
			j.Run(t.Context())
			require.NoError(t, j.Error())

			dbDistro, err := distro.FindOneId(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDistro)
			assert.Greater(t, dbDistro.HostAllocatorSettings.MaximumHosts, originalMaxHosts, "max hosts should be increased due to hitting max hosts occasionally")

			events, err := event.FindAllByResourceID(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.Len(t, events, 1)
			assert.Equal(t, event.ResourceTypeDistro, events[0].ResourceType)
			assert.Equal(t, event.EventDistroModified, events[0].EventType)
		},
		"DecreasesMaxHostsWhenMaxHostUtilizationIsVeryLow": func(t *testing.T, env *mock.Environment, j *distroAutoTuneJob) {
			originalMaxHosts := j.distro.HostAllocatorSettings.MaximumHosts
			require.NoError(t, j.distro.Insert(t.Context()))

			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{1}, 10)...)
			j.Run(t.Context())
			require.NoError(t, j.Error())

			dbDistro, err := distro.FindOneId(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDistro)
			assert.Less(t, dbDistro.HostAllocatorSettings.MaximumHosts, originalMaxHosts, "max hosts should be decreased due to using very few hosts")

			events, err := event.FindAllByResourceID(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.Len(t, events, 1)
			assert.Equal(t, event.ResourceTypeDistro, events[0].ResourceType)
			assert.Equal(t, event.EventDistroModified, events[0].EventType)
		},
		"NoopsForNonEC2Distro": func(t *testing.T, env *mock.Environment, j *distroAutoTuneJob) {
			originalMaxHosts := j.distro.HostAllocatorSettings.MaximumHosts
			j.distro.Provider = evergreen.ProviderNameStatic
			require.NoError(t, j.distro.Insert(t.Context()))

			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{j.distro.HostAllocatorSettings.MaximumHosts}, 10)...)
			j.Run(t.Context())
			require.NoError(t, j.Error())

			dbDistro, err := distro.FindOneId(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDistro)
			assert.Equal(t, originalMaxHosts, dbDistro.HostAllocatorSettings.MaximumHosts, "max hosts should not change for non-EC2 distro")
		},
		"NoopsIfDistroAutoTuningDisabled": func(t *testing.T, env *mock.Environment, j *distroAutoTuneJob) {
			j.distro.HostAllocatorSettings.AutoTuneMaximumHosts = false
			require.NoError(t, j.distro.Insert(t.Context()))

			originalMaxHosts := j.distro.HostAllocatorSettings.MaximumHosts
			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{j.distro.HostAllocatorSettings.MaximumHosts}, 10)...)
			j.Run(t.Context())
			require.NoError(t, j.Error())

			dbDistro, err := distro.FindOneId(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDistro)
			assert.Equal(t, originalMaxHosts, dbDistro.HostAllocatorSettings.MaximumHosts, "max hosts should not change if auto-tuning disabled")
		},
		"NoopsForSingleTaskDistro": func(t *testing.T, env *mock.Environment, j *distroAutoTuneJob) {
			j.distro.SingleTaskDistro = true
			require.NoError(t, j.distro.Insert(t.Context()))

			originalMaxHosts := j.distro.HostAllocatorSettings.MaximumHosts
			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{j.distro.HostAllocatorSettings.MaximumHosts}, 10)...)
			j.Run(t.Context())
			require.NoError(t, j.Error())

			dbDistro, err := distro.FindOneId(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDistro)
			assert.Equal(t, originalMaxHosts, dbDistro.HostAllocatorSettings.MaximumHosts, "max hosts should not change for single task distro")
		},
		"NoopsForNoHostStats": func(t *testing.T, env *mock.Environment, j *distroAutoTuneJob) {
			originalMaxHosts := j.distro.HostAllocatorSettings.MaximumHosts
			require.NoError(t, j.distro.Insert(t.Context()))

			j.Run(t.Context())
			require.NoError(t, j.Error())

			dbDistro, err := distro.FindOneId(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDistro)
			assert.Equal(t, originalMaxHosts, dbDistro.HostAllocatorSettings.MaximumHosts, "max hosts should not change if there's no host data")
		},
		"DoesNotChangeMaxHostsForExtremelyLowAmountOfTimeUsingHosts": func(t *testing.T, env *mock.Environment, j *distroAutoTuneJob) {
			originalMaxHosts := j.distro.HostAllocatorSettings.MaximumHosts
			require.NoError(t, j.distro.Insert(t.Context()))

			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{0}, 200)...)
			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{1}, 1)...)
			j.Run(t.Context())
			require.NoError(t, j.Error())

			dbDistro, err := distro.FindOneId(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDistro)
			assert.Equal(t, originalMaxHosts, dbDistro.HostAllocatorSettings.MaximumHosts, "max hosts should not change if the distro is rarely used")
		},
		"CannotDecreaseMaxHostsBelowMinHosts": func(t *testing.T, env *mock.Environment, j *distroAutoTuneJob) {
			j.distro.HostAllocatorSettings.MinimumHosts = j.distro.HostAllocatorSettings.MaximumHosts - 1
			require.NoError(t, j.distro.Insert(t.Context()))

			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{1}, 10)...)
			j.Run(t.Context())
			require.NoError(t, j.Error())

			dbDistro, err := distro.FindOneId(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDistro)
			assert.Equal(t, j.distro.HostAllocatorSettings.MinimumHosts, dbDistro.HostAllocatorSettings.MaximumHosts, "max hosts should be decreased due to using very few hosts but not go below min hosts")

			events, err := event.FindAllByResourceID(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.Len(t, events, 1)
			assert.Equal(t, event.ResourceTypeDistro, events[0].ResourceType)
			assert.Equal(t, event.EventDistroModified, events[0].EventType)
		},
		"DoesNotChangeMaxHostsIfMaxHostsIsOccasionallyHit": func(t *testing.T, env *mock.Environment, j *distroAutoTuneJob) {
			originalMaxHosts := j.distro.HostAllocatorSettings.MaximumHosts
			require.NoError(t, j.distro.Insert(t.Context()))

			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{j.distro.HostAllocatorSettings.MaximumHosts / 2}, 100)...)
			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{j.distro.HostAllocatorSettings.MaximumHosts}, 1)...)
			j.Run(t.Context())
			require.NoError(t, j.Error())

			dbDistro, err := distro.FindOneId(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDistro)
			assert.Equal(t, originalMaxHosts, dbDistro.HostAllocatorSettings.MaximumHosts, "max hosts should not be changed if it occasionally hits max hosts")
		},
		"DoesNotChangeMaxHostsIfMaxHostsIsOccasionallyAlmostHIt": func(t *testing.T, env *mock.Environment, j *distroAutoTuneJob) {
			originalMaxHosts := j.distro.HostAllocatorSettings.MaximumHosts
			require.NoError(t, j.distro.Insert(t.Context()))

			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{j.distro.HostAllocatorSettings.MaximumHosts / 2}, 100)...)
			addDistroHostStats(t, j.distro.Id, slices.Repeat([]int{j.distro.HostAllocatorSettings.MaximumHosts - 1}, 1)...)
			j.Run(t.Context())
			require.NoError(t, j.Error())

			dbDistro, err := distro.FindOneId(t.Context(), j.distro.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDistro)
			assert.Equal(t, originalMaxHosts, dbDistro.HostAllocatorSettings.MaximumHosts, "max hosts should not be changed if it occasionally is close to hitting max hosts")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(colls...))
			env := &mock.Environment{}
			require.NoError(t, env.Configure(t.Context()))
			env.EvergreenSettings.HostInit.MaxTotalDynamicHosts = 10000

			d := &distro.Distro{
				Id:       "d",
				Provider: evergreen.ProviderNameEc2Fleet,
				HostAllocatorSettings: distro.HostAllocatorSettings{
					MaximumHosts:         100,
					AutoTuneMaximumHosts: true,
				},
			}

			j := makeDistroAutoTuneJob()
			j.settings = env.Settings()
			j.distro = d
			j.DistroID = d.Id
			tCase(t, env, j)
		})
	}
}
