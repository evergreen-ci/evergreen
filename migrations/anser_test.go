package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/anser"
	"github.com/mongodb/anser/mock"
	"github.com/stretchr/testify/assert"
)

func TestAnserBasicPlaceholder(t *testing.T) {
	assert := assert.New(t)
	opts := Options{
		Database: "mci_test",
		URI:      "mongodb://localhost:27017",
		Period:   time.Second,
		Target:   2,
		Workers:  2,
	}

	app, err := opts.Application(mock.NewEnvironment())
	assert.NoError(err)
	assert.False(app.DryRun)

	env, err := opts.Setup(context.Background())
	assert.NoError(err)
	assert.NotNil(env)
	assert.NoError(env.Close())

	anser.ResetEnvironment()

	// will use default
	env, err = opts.Setup(context.Background())
	assert.NoError(err)
	assert.NotNil(env)
	assert.NoError(env.Close())
}
