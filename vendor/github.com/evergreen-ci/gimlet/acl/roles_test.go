package acl

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/stretchr/testify/assert"
)

func TestRoleRouteHandlers(t *testing.T) {
	m := rolemanager.NewInMemoryRoleManager()
	t.Run("TestRoleUpdate", testRoleUpdate(t, m))
	t.Run("TestRoleRead", testRoleRead(t, m))
}

func testRoleUpdate(t *testing.T, m gimlet.RoleManager) func(t *testing.T) {
	return func(t *testing.T) {
		body := map[string]interface{}{
			"id":          "myRole",
			"permissions": map[string]int{"p1": 1},
			"owners":      []string{"me"},
		}
		var validateWasCalled bool
		validate := func(gimlet.Role) error {
			validateWasCalled = true
			return nil
		}
		handler := NewUpdateRoleHandler(m, validate)

		jsonBody, err := json.Marshal(body)
		assert.NoError(t, err)
		buffer := bytes.NewBuffer(jsonBody)
		request, err := http.NewRequest(http.MethodPost, "/roles", buffer)
		assert.NoError(t, handler.Parse(context.Background(), request))
		assert.True(t, validateWasCalled)
		resp := handler.Run(context.Background())
		assert.Equal(t, 200, resp.Status())
	}
}

func testRoleRead(t *testing.T, m gimlet.RoleManager) func(t *testing.T) {
	return func(t *testing.T) {
		handler := NewGetAllRolesHandler(m)
		assert.NoError(t, handler.Parse(context.Background(), nil))
		resp := handler.Run(context.Background())
		roles, valid := resp.Data().([]gimlet.Role)
		assert.True(t, valid)
		assert.Equal(t, "myRole", roles[0].ID)
	}
}
