package migrations

import (
	"context"
	"testing"
	"time"

	evg "github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/mock"
	"github.com/stretchr/testify/assert"
)

func TestAnserBasicPlaceholder(t *testing.T) {
	assert := assert.New(t) // nolint
	mgoSession, _, err := evg.GetGlobalSessionFactory().GetSession()
	assert.NoError(err)
	session := db.WrapSession(mgoSession.Clone())
	defer session.Close()

	anser.ResetEnvironment()

	opts := Options{
		Database: "mci_test",
		Period:   time.Second,
		Target:   2,
		Workers:  2,
		Session:  session,
	}

	app, err := opts.Application(mock.NewEnvironment())
	assert.NoError(err)
	assert.False(app.DryRun)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env, err := opts.Setup(ctx)
	assert.NoError(err)
	assert.NotNil(env)
	assert.NoError(env.Close())

	anser.ResetEnvironment()

	// will use default
	env, err = opts.Setup(context.Background())
	assert.NoError(err)
	assert.NotNil(env)
	assert.NoError(env.Close())

	anser.ResetEnvironment()

}
