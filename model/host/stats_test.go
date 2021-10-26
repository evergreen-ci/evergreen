package host

import (
	"sort"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/stretchr/testify/assert"
)

func insertTestDocuments() error {
	input := []interface{}{
		Host{
			Id:     "one",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "debian",
				Provider: evergreen.ProviderNameEc2Auto,
			},
			RunningTask: "baz",
			StartedBy:   evergreen.User,
		},
		Host{
			Id:     "two",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "redhat",
				Provider: evergreen.ProviderNameEc2Auto,
			},
			RunningTask: "bar",
			StartedBy:   evergreen.User,
		},
		Host{
			Id:     "three",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "debian",
				Provider: evergreen.ProviderNameEc2Auto,
			},
			RunningTask: "foo-foo",
			StartedBy:   evergreen.User,
		},
		Host{
			Id:     "four",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "redhat",
				Provider: evergreen.ProviderNameEc2Spot,
			},
			StartedBy: evergreen.User,
		},
		Host{
			Id:     "five",
			Status: evergreen.HostUninitialized,
			Distro: distro.Distro{
				Id:       "foo",
				Provider: evergreen.ProviderNameEc2Auto,
			},
			StartedBy: evergreen.User,
		},
		Host{
			Id:     "six",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "bar",
				Provider: evergreen.ProviderNameStatic,
			},
			StartedBy: evergreen.User,
		},
		Host{
			Id:     "seven",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "debian",
				Provider: evergreen.ProviderNameStatic,
			},
			RunningTask: "foo",
		},
	}

	return db.InsertMany(Collection, input...)
}

func TestHostStatsByProvider(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	defer func() {
		assert.NoError(db.ClearCollections(Collection))
	}()
	assert.NoError(insertTestDocuments())

	result := ProviderStats{}

	assert.NoError(db.Aggregate(Collection, statsByProviderPipeline(), &result))
	assert.Len(result, 3, "%+v", result)

	rmap := result.Map()
	assert.Equal(1, rmap[evergreen.ProviderNameEc2Spot])
	assert.Equal(3, rmap[evergreen.ProviderNameEc2Auto])

	alt, err := GetProviderCounts()
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
	assert.NoError(insertTestDocuments())

	result := DistroStats{}

	assert.NoError(db.Aggregate(Collection, statsByDistroPipeline(), &result))
	assert.Len(result, 3, "%+v", result)

	rcmap := result.CountMap()
	assert.Equal(2, rcmap["debian"])
	assert.Equal(2, rcmap["redhat"])

	rtmap := result.TasksMap()
	assert.Equal(2, rtmap["debian"])
	assert.Equal(1, rtmap["redhat"])

	exceeded := result.MaxHostsExceeded()
	assert.Len(exceeded, 2)
	assert.NotContains(exceeded, "bar")

	alt, err := GetStatsByDistro()
	assert.NoError(err)
	sort.Slice(alt, func(i, j int) bool { return alt[i].Distro < alt[j].Distro })
	sort.Slice(result, func(i, j int) bool { return result[i].Distro < result[j].Distro })
	assert.Equal(alt, result)
}
