package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
)

func TestAdminFlagsRouteSuite(t *testing.T) {
	assert := assert.New(t)
	sc := &data.MockConnector{}

	postHandler := makeSetServiceFlagsRouteManager(sc)
	assert.NotNil(postHandler)

	// run the route
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

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
	assert.NoError(postHandler.Parse(ctx, request))
	h := postHandler.(*flagsPostHandler)
	assert.Equal(body.Flags, h.Flags)

	// test executing the POST request
	resp := postHandler.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	settings, err := sc.GetEvergreenSettings()
	assert.NoError(err)
	assert.Equal(body.Flags.HostinitDisabled, settings.ServiceFlags.HostinitDisabled)
	assert.Equal(body.Flags.TaskrunnerDisabled, settings.ServiceFlags.TaskrunnerDisabled)
	assert.Equal(body.Flags.RepotrackerDisabled, settings.ServiceFlags.RepotrackerDisabled)
}
