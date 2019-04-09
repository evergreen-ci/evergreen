package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/stretchr/testify/assert"
)

func TestAPITestStatsBuildFromService(t *testing.T) {
	assert := assert.New(t)

	serviceDoc := stats.TestStats{
		TestFile:     "test1",
		TaskName:     "task1",
		BuildVariant: "variant1",
		Distro:       "distro1",
		Date:         time.Now().UTC(),

		NumPass:         17,
		NumFail:         99,
		AvgDurationPass: float64(12.34),
	}

	apiDoc := APITestStats{}
	err := apiDoc.BuildFromService(&serviceDoc)
	assert.NoError(err)

	assert.Equal(serviceDoc.TestFile, *apiDoc.TestFile)
	assert.Equal(serviceDoc.TaskName, *apiDoc.TaskName)
	assert.Equal(serviceDoc.BuildVariant, *apiDoc.BuildVariant)
	assert.Equal(serviceDoc.Distro, *apiDoc.Distro)
	assert.Equal(serviceDoc.Date.Format("2006-01-02"), *apiDoc.Date)
	assert.Equal(serviceDoc.NumPass, apiDoc.NumPass)
	assert.Equal(serviceDoc.NumFail, apiDoc.NumFail)
	assert.Equal(serviceDoc.AvgDurationPass, apiDoc.AvgDurationPass)
}

func TestAPITestStatsStartAtKey(t *testing.T) {
	assert := assert.New(t)

	apiDoc := APITestStats{
		TestFile:     ToAPIString("test1"),
		TaskName:     ToAPIString("task1"),
		BuildVariant: ToAPIString("variant1"),
		Distro:       ToAPIString("distro1"),
		Date:         ToAPIString("2018-07-15"),
	}
	assert.Equal("2018-07-15|variant1|task1|test1|distro1", apiDoc.StartAtKey())

	apiDoc = APITestStats{
		TestFile: ToAPIString("test1"),
		Date:     ToAPIString("2018-07-15"),
	}
	assert.Equal("2018-07-15|||test1|", apiDoc.StartAtKey())
}

func TestAPITaskStatsBuildFromService(t *testing.T) {
	assert := assert.New(t)

	serviceDoc := stats.TaskStats{
		TaskName:     "task1",
		BuildVariant: "variant1",
		Distro:       "distro1",
		Date:         time.Now().UTC(),

		NumSuccess:         17,
		NumFailed:          99,
		NumTotal:           116,
		NumTimeout:         3,
		NumTestFailed:      44,
		NumSystemFailed:    55,
		NumSetupFailed:     13,
		AvgDurationSuccess: float64(12.34),
	}

	apiDoc := APITaskStats{}
	err := apiDoc.BuildFromService(&serviceDoc)
	assert.NoError(err)

	assert.Equal(serviceDoc.TaskName, *apiDoc.TaskName)
	assert.Equal(serviceDoc.BuildVariant, *apiDoc.BuildVariant)
	assert.Equal(serviceDoc.Distro, *apiDoc.Distro)
	assert.Equal(serviceDoc.Date.Format("2006-01-02"), *apiDoc.Date)
	assert.Equal(serviceDoc.NumSuccess, apiDoc.NumSuccess)
	assert.Equal(serviceDoc.NumFailed, apiDoc.NumFailed)
	assert.Equal(serviceDoc.NumTotal, apiDoc.NumTotal)
	assert.Equal(serviceDoc.NumTimeout, apiDoc.NumTimeout)
	assert.Equal(serviceDoc.NumTestFailed, apiDoc.NumTestFailed)
	assert.Equal(serviceDoc.NumSystemFailed, apiDoc.NumSystemFailed)
	assert.Equal(serviceDoc.NumSetupFailed, apiDoc.NumSetupFailed)
	assert.Equal(serviceDoc.AvgDurationSuccess, apiDoc.AvgDurationSuccess)
}

func TestAPITaskStatsStartAtKey(t *testing.T) {
	assert := assert.New(t)

	apiDoc := APITaskStats{
		TaskName:     ToAPIString("task1"),
		BuildVariant: ToAPIString("variant1"),
		Distro:       ToAPIString("distro1"),
		Date:         ToAPIString("2018-07-15"),
	}
	assert.Equal("2018-07-15|variant1|task1||distro1", apiDoc.StartAtKey())

	apiDoc = APITaskStats{
		TaskName: ToAPIString("task1"),
		Date:     ToAPIString("2018-07-15"),
	}
	assert.Equal("2018-07-15||task1||", apiDoc.StartAtKey())
}
