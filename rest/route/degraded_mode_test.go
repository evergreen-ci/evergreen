package route

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetDegradedMode(t *testing.T) {
	assert := assert.New(t)

	routeManager := makeSetDegradedMode()
	assert.NotNil(routeManager)
	assert.IsType(&degradedModeHandler{}, routeManager)

	ctx := context.Background()
	var settings evergreen.Settings
	require.NoError(t, settings.Get(ctx))
	originalFlags, err := evergreen.GetServiceFlags(ctx)
	require.NoError(t, err)
	// Since the tests depend on modifying the global environment, reset it to
	// its initial state afterwards.
	defer func() {
		require.NoError(t, settings.Set(ctx))
		require.NoError(t, originalFlags.Set(ctx))
	}()

	settings.TaskLimits.MaxParserProjectSize = 18
	settings.TaskLimits.MaxDegradedModeParserProjectSize = 16
	settings.Banner = ""
	originalFlags.CPUDegradedModeDisabled = true
	require.NoError(t, settings.Set(ctx))
	require.NoError(t, originalFlags.Set(ctx))

	assert.True(originalFlags.CPUDegradedModeDisabled)
	json := []byte(`{
    "receiver": "webhook-devprod-evergreen",
    "status": "firing"
    }`)
	request, err := http.NewRequest(http.MethodPost, "/degraded_mode", bytes.NewBuffer(json))
	assert.NoError(err)
	err = routeManager.Parse(ctx, request)
	assert.NoError(err)
	resp := routeManager.Run(ctx)
	assert.NoError(err)
	require.NotNil(t, resp)
	assert.Equal(http.StatusOK, resp.Status())

	dbSettings, err := evergreen.GetConfig(ctx)
	require.NotNil(t, dbSettings)
	assert.NoError(err)
	assert.False(dbSettings.ServiceFlags.CPUDegradedModeDisabled)
	assert.Equal(evergreen.Information, dbSettings.BannerTheme)
	msg = fmt.Sprintf("Evergreen is under high load, max config YAML size has been reduced from %dMB to %dMB. "+
		"Existing tasks with large (>16MB) config YAMLs may also experience slower scheduling.", dbSettings.TaskLimits.MaxParserProjectSize, dbSettings.TaskLimits.MaxDegradedModeParserProjectSize)
	assert.Equal(msg, dbSettings.Banner)

	json = []byte(`{
    "receiver": "webhook-devprod-evergreen",
    "status": "resolved"
    }`)
	request, err = http.NewRequest(http.MethodPost, "/degraded_mode", bytes.NewBuffer(json))
	assert.NoError(err)
	err = routeManager.Parse(ctx, request)
	assert.Error(err)

}
