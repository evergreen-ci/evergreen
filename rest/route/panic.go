package route

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// GET /rest/v2/panic
// This route is used to report a panic to the Evergreen service.
type panicReport struct {
	report *restModel.PanicReport

	env evergreen.Environment
}

func makePanicReport(env evergreen.Environment) gimlet.RouteHandler {
	return &panicReport{env: env}
}

func (h *panicReport) Factory() gimlet.RouteHandler {
	return &panicReport{env: h.env}
}

func (h *panicReport) Parse(ctx context.Context, r *http.Request) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.Wrap(err, "reading body")
	}

	return errors.Wrap(json.Unmarshal(body, &h.report), "unmarshalling panic report")
}

func (h *panicReport) Run(ctx context.Context) gimlet.Responder {
	grip.Error(message.Fields{
		"message": "CLI panic report",
		"report":  h.report,
	})
	return gimlet.NewTextResponse(nil)
}
