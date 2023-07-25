package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
)

func TestAdminFlagsRouteSuite(t *testing.T) {
	assert := assert.New(t)

	postHandler := makeSetServiceFlagsRouteManager()
	assert.NotNil(postHandler)

	// run the route
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	// test parsing the POST body
	body := struct {
		Flags model.APIServiceFlags `json:"service_flags"`
	}{
		Flags: model.APIServiceFlags{
			HostInitDisabled:   true,
			AgentStartDisabled: true,
		},
	}
	jsonBody, err := json.Marshal(&body)
	assert.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/admin/service_flags", buffer)
	assert.NoError(err)
	assert.NoError(postHandler.Parse(ctx, request))
	h := postHandler.(*flagsPostHandler)
	assert.Equal(body.Flags, h.Flags)

	// test executing the POST request
	resp := postHandler.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	settings, err := evergreen.GetConfig(ctx)
	assert.NoError(err)
	assert.Equal(body.Flags.HostInitDisabled, settings.ServiceFlags.HostInitDisabled)
	assert.Equal(body.Flags.AgentStartDisabled, settings.ServiceFlags.AgentStartDisabled)
	assert.Equal(body.Flags.RepotrackerDisabled, settings.ServiceFlags.RepotrackerDisabled)
}
