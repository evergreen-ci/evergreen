package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
)

func TestAllRegisteredUnitsAreRemoteSafe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testutil.TestSpan(ctx, t)

	assert := assert.New(t)

	disabled := []string{
		"bond-recall-download-file",
	}
	for id := range registry.JobTypeNames() {
		if utility.StringSliceContains(disabled, id) {
			continue
		}
		grip.Infoln("testing job is remote ready:", id)
		factory, err := registry.GetJobFactory(id)
		assert.NoError(err)
		assert.NotNil(factory)
		job := factory()

		assert.NotNil(job)

		assert.Equal(id, job.Type().Name)
		for _, f := range []amboy.Format{amboy.JSON, amboy.JSON} {
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
