package route

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceFlagsGetHandler(t *testing.T) {
	ctx := t.Context()
	originalFlags, err := evergreen.GetServiceFlags(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, originalFlags.Set(context.WithoutCancel(ctx)))
	})

	flags := *originalFlags
	flags.DebugSpawnHostDisabled = true
	flags.CrossFileYAMLAnchorsEnabled = true
	require.NoError(t, flags.Set(ctx))

	resp := makeFetchServiceFlags().Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status())

	data, err := json.Marshal(resp.Data())
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"debug_spawn_host_disabled": true,
		"cross_file_yaml_anchors_enabled": true
	}`, string(data))
}
