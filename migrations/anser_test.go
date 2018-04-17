package migrations

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	evg "github.com/evergreen-ci/evergreen/db"
	evgmock "github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/mock"
	"github.com/stretchr/testify/assert"
)

func TestAnserBasicPlaceholder(t *testing.T) {
	assert := assert.New(t)
	mgoSession, _, err := evg.GetGlobalSessionFactory().GetSession()
	assert.NoError(err)
	session := db.WrapSession(mgoSession.Clone())
	defer session.Close()

	anser.ResetEnvironment()

	evgEnv := &evgmock.Environment{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(evgEnv.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))

	opts := Options{
		Database: "mci_test",
		Period:   time.Second,
		Target:   2,
		Workers:  2,
		Session:  session,
	}

	app, err := opts.Application(mock.NewEnvironment(), evgEnv)
	assert.NoError(err)
	assert.NotNil(app)

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
