package units

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestSpawnhostModifyJob(t *testing.T) {
	config := testutil.TestConfig()
	assert.NoError(t, evergreen.UpdateConfig(config))
	assert.NoError(t, db.ClearCollections(host.Collection))
	h := host.Host{
		Id:           "hostID",
		Provider:     evergreen.ProviderNameMock,
		InstanceTags: map[string]string{"key1": "value1"},
		Distro:       distro.Distro{Provider: evergreen.ProviderNameMock},
	}
	assert.NoError(t, h.Insert())

	changes := host.HostModifyOptions{
		AddInstanceTags:    map[string]string{"key2": "value2"},
		DeleteInstanceTags: []string{"key1"},
	}

	j := makeSpawnhostModifyJob()
	j.host = &h
	j.changes = changes

	ctx := context.Background()
	env := &mock.Environment{}
	j.env = env
	assert.NoError(t, env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))

	j.Run(ctx)
	assert.NoError(t, j.Error())
	assert.True(t, j.Status().Completed)

	modifiedHost, err := host.FindOneId(h.Id)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"key2": "value2"}, modifiedHost.InstanceTags)
}
