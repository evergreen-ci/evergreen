package user

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestBasicDBFunctions(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(RoleCollection))
	r := Role{
		Id:          "1",
		Name:        "foo",
		Scope:       "proj",
		ScopeType:   ScopeTypeProject,
		Permissions: map[string]string{"a": "b"},
	}
	_, err := r.Upsert()
	assert.NoError(err)
	dbRole, err := FindOneRoleId(r.Id)
	assert.NoError(err)
	assert.EqualValues(r, *dbRole)
}
