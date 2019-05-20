package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
)

func makeVersionCreateHandler(sc data.Connector) gimlet.RouteHandler {
	return &versionCreateHandler{sc: sc}
}

type versionCreateHandler struct {
	ProjectID string          `json:"project_id"`
	Message   string          `json:"message"`
	Active    bool            `json:"activate"`
	Config    json.RawMessage `json:"config"`

	sc data.Connector
}

func (h *versionCreateHandler) Factory() gimlet.RouteHandler {
	return &versionCreateHandler{sc: h.sc}
}

func (h *versionCreateHandler) Parse(ctx context.Context, r *http.Request) error {
	err := util.ReadJSONInto(r.Body, h)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("error parsing request body: %s", err.Error()),
		}
	}
	return nil
}

func (h *versionCreateHandler) Run(ctx context.Context) gimlet.Responder {
	u := gimlet.GetUser(ctx).(*user.DBUser)
	newVersion, err := h.sc.CreateVersionFromConfig(ctx, h.ProjectID, h.Config, u, h.Message, h.Active)
	if err != nil {
		return gimlet.NewJSONErrorResponse(err)
	}
	return gimlet.NewJSONResponse(newVersion)
}
