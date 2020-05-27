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

func TestMergeAppsIntoRoute(t *testing.T) {
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

func TestMergeApps(t *testing.T) {
	// one app should merge just fine
	app := NewApp()
	app.AddMiddleware(MakeRecoveryLogger())

	err := app.Merge(app)
	assert.NoError(t, err)

	// can't merge more than once
	err = app.Merge(app)
	assert.Error(t, err)
	app.hasMerged = false

	err = app.Merge()
	assert.Error(t, err)
	app.hasMerged = false

	// check duplicate without prefix or middleware
	app.AddRoute("/foo").Version(2).Get()
	bad := NewApp()
	bad.AddRoute("/foo").Version(2).Get()
	err = app.Merge(bad)
	assert.Error(t, err)
	app.hasMerged = false

	app.SetPrefix("foo")
	assert.Error(t, app.Merge(bad))
	app.prefix = ""

	app2 := NewApp()
	app2.SetPrefix("foo")
	app2.AddMiddleware(MakeRecoveryLogger())

	err = app.Merge(app2)
	assert.NoError(t, err)
	app.hasMerged = false

	err = app.Merge(app2, app2)
	assert.Error(t, err)
	app.hasMerged = false

	// can't have duplicated methods
	app2 = NewApp()
	app2.SetPrefix("foo")
	app2.AddRoute("/foo").Version(2).Get()
	err = app.Merge(app2)
	assert.NoError(t, err)
	app.hasMerged = false

	app3 := NewApp()
	app3.AddMiddleware(MakeRecoveryLogger())
	app3.SetPrefix("/wat")
	app3.AddRoute("/bar").Version(3).Get()
	err = app.Merge(app3)
	assert.NoError(t, err)
	app.hasMerged = false

	app2.AddMiddleware(MakeRecoveryLogger())
	app3.prefix = ""
	err = app.Merge(app2, app3)
	assert.Error(t, err)
	app.hasMerged = false

}

func TestAppContainsRoute(t *testing.T) {
	app := NewApp()
	app.AddRoute("/foo").Get().Version(2)

	assert.False(t, app.containsRoute("/foo", 1, []httpMethod{patch}))
	assert.False(t, app.containsRoute("/foo", 2, []httpMethod{patch}))
	assert.False(t, app.containsRoute("/bar", 1, []httpMethod{get}))
	assert.False(t, app.containsRoute("/bar", 2, []httpMethod{get}))
	assert.False(t, app.containsRoute("/foo", 1, []httpMethod{get}))
	assert.True(t, app.containsRoute("/foo", 2, []httpMethod{get}))
}
