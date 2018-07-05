package gimlet

import (
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func TestAssembleHandler(t *testing.T) {
	assert := assert.New(t)
	router := mux.NewRouter()

	app := NewApp()
	app.AddRoute("/foo").version = -1

	h, err := AssembleHandler(router, app)
	assert.Error(err)
	assert.Nil(h)

	app = NewApp()
	app.SetPrefix("foo")
	app.AddMiddleware(MakeRecoveryLogger())

	h, err = AssembleHandler(router, app)
	assert.NoError(err)
	assert.NotNil(h)

	// can't have duplicated methods
	app2 := NewApp()
	app2.SetPrefix("foo")
	h, err = AssembleHandler(router, app, app2)
	assert.Error(err)
	assert.Nil(h)

	app = NewApp()
	app.AddMiddleware(MakeRecoveryLogger())
	h, err = AssembleHandler(router, app)
	assert.NoError(err)
	assert.NotNil(h)
}

func TestMergeApps(t *testing.T) {
	// error when no apps
	h, err := MergeApplications()
	assert.Error(t, err)
	assert.Nil(t, h)

	// one app should merge just fine
	app := NewApp()
	app.SetPrefix("foo")
	app.AddMiddleware(MakeRecoveryLogger())

	h, err = MergeApplications(app)
	assert.NoError(t, err)
	assert.NotNil(t, h)

	// a bad app should error
	bad := NewApp()
	bad.AddRoute("/foo").version = -1

	h, err = MergeApplications(bad)
	assert.Error(t, err)
	assert.Nil(t, h)

	// even when it's combined with a good one
	h, err = MergeApplications(bad, app)
	assert.Error(t, err)
	assert.Nil(t, h)
}
