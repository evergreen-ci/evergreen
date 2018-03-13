package units

import (
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
)

func TestAllRegisteredUnitsAreRemoteSafe(t *testing.T) {
	assert := assert.New(t)

	for id := range registry.JobTypeNames() {
		grip.Infoln("testing job is remote ready:", id)
		factory, err := registry.GetJobFactory(id)
		assert.NoError(err)
		assert.NotNil(factory)
		job := factory()

		assert.NotNil(job)

		assert.Equal(id, job.Type().Name)

		for _, f := range []amboy.Format{amboy.JSON, amboy.YAML, amboy.JSON} {
			assert.NotPanics(func() {
				dbjob, err := registry.MakeJobInterchange(job, f)

				assert.NoError(err)
				assert.NotNil(dbjob)
				assert.NotNil(dbjob.Dependency)
				assert.Equal(id, dbjob.Type)
			}, id)
		}
	}

}
