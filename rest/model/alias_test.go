package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2/bson"
)

func TestAliasBuildFromService(t *testing.T) {
	d := model.ProjectAlias{
		ID:        bson.NewObjectId(),
		ProjectID: "hai",
		Alias:     "alias",
		Variant:   "variant",
		Task:      "task",
	}
	apiAlias := &APIAlias{}
	err := apiAlias.BuildFromService(d)
	assert.NoError(t, err)
	assert.Equal(t, FromApiString(apiAlias.Alias), d.Alias)
	assert.Equal(t, FromApiString(apiAlias.Variant), d.Variant)
	assert.Equal(t, FromApiString(apiAlias.Task), d.Task)
}
