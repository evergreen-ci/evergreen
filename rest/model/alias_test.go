package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
)

func TestAliasBuildFromService(t *testing.T) {
	d := model.ProjectAlias{
		ID:        bson.NewObjectId(),
		ProjectID: "hai",
		Alias:     "alias",
		Variant:   "variant",
		Task:      "task",
		Tags:      []string{"tag1", "tag2"},
	}
	apiAlias := &APIAlias{}
	err := apiAlias.BuildFromService(d)
	assert.NoError(t, err)
	assert.Equal(t, FromAPIString(apiAlias.Alias), d.Alias)
	assert.Equal(t, FromAPIString(apiAlias.Variant), d.Variant)
	assert.Equal(t, FromAPIString(apiAlias.Task), d.Task)
	for i, tag := range apiAlias.Tags {
		assert.Equal(t, FromAPIString(tag), d.Tags[i])
	}
}
