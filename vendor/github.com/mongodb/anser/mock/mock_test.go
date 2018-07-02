package mock

import (
	"testing"

	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/stretchr/testify/assert"
)

func TestInterfaces(t *testing.T) {
	assert := assert.New(t)

	assert.Implements((*db.Session)(nil), &Session{})
	assert.Implements((*db.Database)(nil), &Database{})
	assert.Implements((*db.Collection)(nil), &Collection{})
	assert.Implements((*db.Query)(nil), &Query{})
	assert.Implements((*db.Results)(nil), &Query{})
	assert.Implements((*db.Results)(nil), &Pipeline{})
	assert.Implements((*db.Iterator)(nil), &Iterator{})

	assert.Implements((*model.DependencyNetworker)(nil), &DependencyNetwork{})
	assert.Implements((*anser.Environment)(nil), &Environment{})
	assert.Implements((*dependency.Manager)(nil), &DependencyManager{})
	assert.Implements((*db.BufferedInserter)(nil), &BufferedInserter{})
}
