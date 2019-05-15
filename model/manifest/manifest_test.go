package manifest

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"

	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestUpdateModuleRev(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	m := Manifest{
		Id: "m",
		Modules: map[string]*Module{
			"foo": &Module{
				Revision: "123",
			},
		},
	}
	_, err := m.TryInsert()
	assert.NoError(err)

	assert.EqualError(m.UpdateModuleRevision("bar", "abc"), "no module named bar found")
	assert.NoError(m.UpdateModuleRevision("foo", "abc"))
	dbManifest, err := FindOne(ById(m.Id))
	assert.NoError(err)
	assert.Equal("abc", dbManifest.Modules["foo"].Revision)
}
