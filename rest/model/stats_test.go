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
		Date:         time.Now(),

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

func TestAPITaskStatsBuildFromService(t *testing.T) {
	assert := assert.New(t)

	serviceDoc := stats.TaskStats{
		TaskName:     "task1",
		BuildVariant: "variant1",
		Distro:       "distro1",
		Date:         time.Now(),

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
