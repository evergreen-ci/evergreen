package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/assert"
)

func TestAdminFlagsRouteSuite(t *testing.T) {
	assert := assert.New(t)
	sc := &data.MockConnector{}

	// test getting the route handler
	const route = "/admin/service_flags"
	const version = 2
	routeManager := getServiceFlagsRouteManager(route, version)
	assert.NotNil(routeManager)
	assert.Equal(route, routeManager.Route)
	assert.Equal(version, routeManager.Version)
	postHandler := routeManager.Methods[0]
	assert.IsType(&flagsPostHandler{}, postHandler.RequestHandler)

	// run the route
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, &user.DBUser{Id: "user"})

	// test parsing the POST body
	body := struct {
		Flags model.APIServiceFlags `json:"service_flags"`
	}{
		Flags: model.APIServiceFlags{
			HostinitDisabled:   true,
			TaskrunnerDisabled: true,
		},
	}
	jsonBody, err := json.Marshal(&body)
	assert.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest("POST", "/admin/service_flags", buffer)
	assert.NoError(err)
	assert.NoError(postHandler.RequestHandler.ParseAndValidate(ctx, request))
	h := postHandler.RequestHandler.(*flagsPostHandler)
	assert.Equal(body.Flags, h.Flags)

	// test executing the POST request
	resp, err := postHandler.RequestHandler.Execute(ctx, sc)
	assert.NoError(err)
	assert.NotNil(resp)
	settings, err := sc.GetAdminSettings()
	assert.NoError(err)
	assert.Equal(body.Flags.HostinitDisabled, settings.ServiceFlags.HostinitDisabled)
	assert.Equal(body.Flags.TaskrunnerDisabled, settings.ServiceFlags.TaskrunnerDisabled)
	assert.Equal(body.Flags.RepotrackerDisabled, settings.ServiceFlags.RepotrackerDisabled)
}
