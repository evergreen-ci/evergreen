package cloud

import (
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
)

func TestGetPoolSize(t *testing.T) {
	settings := StaticSettings{
		Hosts: []StaticHost{{Name: "host1"}, {Name: "host2"}, {Name: "host3"}},
	}
	bytes, err := bson.Marshal(settings)
	assert.NoError(t, err)
	doc := birch.Document{}
	assert.NoError(t, doc.UnmarshalBSON(bytes))
	d := distro.Distro{
		Provider:             evergreen.ProviderNameStatic,
		ProviderSettingsList: []*birch.Document{&doc},
	}
	assert.Equal(t, 3, d.GetPoolSize())
}
