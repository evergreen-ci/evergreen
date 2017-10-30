package migrations

import (
	"context"
	"testing"

	"github.com/mongodb/anser"
	"github.com/mongodb/anser/mock"
	"github.com/stretchr/testify/assert"
)

func TestAnserBasicPlaceholder(t *testing.T) {
	assert := assert.New(t)
	app, err := Application(mock.NewEnvironment(), "mci_test")
	assert.NoError(err)
	assert.False(app.DryRun)

	env, err := Setup(context.Background(), "mongodb://localhost:27017")
	assert.NoError(err)
	assert.NotNil(env)
	assert.NoError(env.Close())

	anser.ResetEnvironment()

	// will use default
	env, err = Setup(context.Background(), "")
	assert.NoError(err)
	assert.NotNil(env)
	assert.NoError(env.Close())

	anser.ResetEnvironment()

	env, err = Setup(context.Background(), "mongodb://localhost:38128")
	assert.Error(err)
	assert.Nil(env)
}
