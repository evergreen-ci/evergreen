package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestAPITaskReliabilityBuildFromService(t *testing.T) {
	assert := assert.New(t)

	serviceDoc := reliability.TaskReliability{
		TaskName:     "task1",
		BuildVariant: "variant1",
		Distro:       "distro1",
		Date:         time.Now().UTC(),

		NumSuccess:         1,
		NumFailed:          2,
		NumTotal:           3,
		NumTimeout:         4,
		NumTestFailed:      5,
		NumSystemFailed:    6,
		NumSetupFailed:     7,
		SuccessRate:        8.0,
		Z:                  9.0,
		AvgDurationSuccess: float64(12.34),
	}

	apiDoc := APITaskReliability{}
	apiDoc.BuildFromService(serviceDoc)

	assert.Equal(serviceDoc.TaskName, *apiDoc.TaskName)
	assert.Equal(serviceDoc.BuildVariant, *apiDoc.BuildVariant)
	assert.Equal(*apiDoc.Distro, serviceDoc.Distro)
	assert.Equal(*apiDoc.Date, serviceDoc.Date.Format("2006-01-02"))
	assert.Equal(apiDoc.NumSuccess, serviceDoc.NumSuccess)
	assert.Equal(apiDoc.NumFailed, serviceDoc.NumFailed)
	assert.Equal(apiDoc.NumTotal, serviceDoc.NumTotal)
	assert.Equal(apiDoc.NumTimeout, serviceDoc.NumTimeout)
	assert.Equal(apiDoc.NumTestFailed, serviceDoc.NumTestFailed)
	assert.Equal(apiDoc.NumSystemFailed, serviceDoc.NumSystemFailed)
	assert.Equal(apiDoc.NumSetupFailed, serviceDoc.NumSetupFailed)
	//nolint:testifylint // We expect it to be exactly equal.
	assert.Equal(apiDoc.SuccessRate, serviceDoc.SuccessRate)
	//nolint:testifylint // We expect it to be exactly equal.
	assert.Equal(apiDoc.AvgDurationSuccess, serviceDoc.AvgDurationSuccess)
	//nolint:testifylint // We expect it to be exactly 0.0.
	assert.Equal(8.0, serviceDoc.SuccessRate)
}

func TestAPITaskReliabilityStartAtKey(t *testing.T) {
	assert := assert.New(t)

	apiDoc := APITaskReliability{
		TaskName:     utility.ToStringPtr("task1"),
		BuildVariant: utility.ToStringPtr("variant1"),
		Distro:       utility.ToStringPtr("distro1"),
		Date:         utility.ToStringPtr("2018-07-15"),
	}
	assert.Equal("2018-07-15|variant1|task1|distro1", apiDoc.StartAtKey())

	apiDoc = APITaskReliability{
		TaskName: utility.ToStringPtr("task1"),
		Date:     utility.ToStringPtr("2018-07-15"),
	}
	assert.Equal("2018-07-15||task1|", apiDoc.StartAtKey())
}
