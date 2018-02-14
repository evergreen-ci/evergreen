package host

import (
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
				Provider: "ec2",
			},
			RunningTask: "foo",
		},
		Host{
			Id:     "two",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "redhat",
				Provider: "ec2",
			},
			RunningTask: "bar",
		},
		Host{
			Id:     "three",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "debian",
				Provider: "ec2",
			},
			RunningTask: "foo-foo",
		},
		Host{
			Id:     "four",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id:       "redhat",
				Provider: "ec2-spot",
			},
		},
		Host{
			Id:     "five",
			Status: evergreen.HostUninitialized,
			Distro: distro.Distro{
				Id:       "foo",
				Provider: "ec2",
			},
		},
	}

	return db.InsertMany(Collection, input...)
}

func TestHostStatsByProvider(t *testing.T) {
	assert := assert.New(t) // nolint
	assert.NoError(db.ClearCollections(Collection))
	assert.NoError(insertTestDocuments())

	result := ProviderStats{}

	assert.NoError(db.Aggregate(Collection, statsByProviderPipeline(), &result))
	assert.Len(result, 2, "%+v", result)

	rmap := result.Map()
	assert.Equal(1, rmap["ec2-spot"])
	assert.Equal(3, rmap["ec2"])

	alt, err := GetProviderCounts()
	assert.NoError(err)
	assert.Equal(alt, result)

	assert.NoError(db.ClearCollections(Collection))
}

func TestHostStatsByDistro(t *testing.T) {
	assert := assert.New(t) // nolint
	assert.NoError(db.ClearCollections(Collection))
	assert.NoError(insertTestDocuments())

	result := DistroStats{}

	assert.NoError(db.Aggregate(Collection, statsByDistroPipeline(), &result))
	assert.Len(result, 2, "%+v", result)

	rcmap := result.CountMap()
	assert.Equal(2, rcmap["debian"])
	assert.Equal(2, rcmap["redhat"])

	rtmap := result.TasksMap()
	assert.Equal(2, rtmap["debian"])
	assert.Equal(1, rtmap["redhat"])

	alt, err := GetStatsByDistro()
	assert.NoError(err)
	assert.Equal(alt, result)

	assert.NoError(db.ClearCollections(Collection))
}
