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
	h, err = AssembleHandler(router, app)
	assert.NoError(err)
	assert.NotNil(h)

	app = NewApp()
	h, err = AssembleHandler(router, app)
	assert.NoError(err)
	assert.NotNil(h)

}
