package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestFetchServiceFlagsAllFieldsReturned verifies that GET /admin/service_flags
// returns every field defined in ServiceFlags. If a new flag is added to
// ServiceFlags but not wired through BuildFromService, this test will fail.
func TestFetchServiceFlagsAllFieldsReturned(t *testing.T) {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	// Build a ServiceFlags with every bool field set to true.
	var allTrue evergreen.ServiceFlags
	rv := reflect.ValueOf(&allTrue).Elem()
	for i := range rv.NumField() {
		f := rv.Field(i)
		if f.Kind() == reflect.Bool {
			f.SetBool(true)
		}
	}
	require.NoError(t, allTrue.Set(ctx))

	getHandler := makeFetchServiceFlags()
	resp := getHandler.Run(ctx)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.Status())

	flags, ok := resp.Data().(*model.APIServiceFlags)
	require.True(t, ok, "response data should be *model.APIServiceFlags")

	// Every bool field in the response must be true — any field that is false
	// was not wired through BuildFromService.
	rv = reflect.ValueOf(flags).Elem()
	for i := range rv.NumField() {
		f := rv.Field(i)
		if f.Kind() == reflect.Bool {
			assert.True(t, f.Bool(), "field %s should be true but was false — check BuildFromService", rv.Type().Field(i).Name)
		}
	}
}
