package route

import (
	"bytes"
	"context"
	"github.com/evergreen-ci/evergreen/testutil"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
)

func TestSetDegradedMode(t *testing.T) {
	assert := assert.New(t)

	routeManager := makeSetDegradedMode()
	assert.NotNil(routeManager)
	assert.IsType(&degradedModeHandler{}, routeManager)

	ctx := context.Background()

	settings := testutil.MockConfig()
	assert.NoError(settings.Set(ctx))
	assert.True(settings.ServiceFlags.DegradedModeDisabled)
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
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	settings, err = evergreen.GetConfig(ctx)
	assert.NoError(err)
	assert.False(settings.ServiceFlags.DegradedModeDisabled)

	json = []byte(`{
    "receiver": "webhook-devprod-evergreen",
    "status": "resolved"
    }`)
	request, err = http.NewRequest(http.MethodPost, "/degraded_mode", bytes.NewBuffer(json))
	assert.NoError(err)
	err = routeManager.Parse(ctx, request)
	assert.Error(err)

}
