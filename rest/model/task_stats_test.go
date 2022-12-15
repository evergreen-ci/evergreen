package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/taskstats"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestAPITaskStatsBuildFromService(t *testing.T) {
	assert := assert.New(t)

	serviceDoc := taskstats.TaskStats{
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
	apiDoc.BuildFromService(serviceDoc)

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
		TaskName:     utility.ToStringPtr("task1"),
		BuildVariant: utility.ToStringPtr("variant1"),
		Distro:       utility.ToStringPtr("distro1"),
		Date:         utility.ToStringPtr("2018-07-15"),
	}
	assert.Equal("2018-07-15|variant1|task1|distro1", apiDoc.StartAtKey())

	apiDoc = APITaskStats{
		TaskName: utility.ToStringPtr("task1"),
		Date:     utility.ToStringPtr("2018-07-15"),
	}
	assert.Equal("2018-07-15||task1|", apiDoc.StartAtKey())
}
