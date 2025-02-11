package route

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
	testselection "github.com/evergreen-ci/test-selection-client"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type selectTestsHandler struct {
	selectTests SelectTestsRequest
	env         evergreen.Environment
}

// SelectTestsRequest represents a request to return a filtered set of tests to
// run. It deliberately includes information that could be looked up in the
// database in order to bypass database lookups. This allows Evergreen to pass
// this information directly to the test selector.
type SelectTestsRequest struct {
	// Project is the project identifier.
	Project string `json:"project"`
	// Requester is the Evergreen requester type.
	Requester string `json:"requester"`
	// BuildVariant is the Evergreen build variant.
	BuildVariant string `json:"build_variant"`
	// TaskID is the Evergreen task ID.
	TaskID string `json:"task_id"`
	// TaskName is the Evergreen task name.
	TaskName string `json:"task_name"`
	// Tests is a list of test names.
	Tests []string `json:"tests"`
}

func makeSelectTestsHandler(env evergreen.Environment) gimlet.RouteHandler {
	return &selectTestsHandler{env: env}
}

// Factory creates an instance of the handler.
//
//	@Summary		Select tests
//	@Description	Return a subset of tests to run for a given task. This endpoint is not yet ready. Please do not use it.
//	@Tags			select
//	@Router			/select/tests [post]
//	@Param			{object}	body	SelectTestsRequest	true	"Select tests request"
//	@Security		Api-User || Api-Key
//	@Success		200	{object}	SelectTestsRequest
func (t *selectTestsHandler) Factory() gimlet.RouteHandler {
	return &selectTestsHandler{env: t.env}
}

func (t *selectTestsHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	defer body.Close()
	b, err := io.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "reading request body")
	}
	if err = json.Unmarshal(b, &t.selectTests); err != nil {
		return errors.Wrap(err, "parsing request body")
	}
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(t.selectTests.Project == "", "project is required")
	catcher.NewWhen(t.selectTests.Requester == "", "requester is required")
	catcher.NewWhen(t.selectTests.BuildVariant == "", "build variant is required")
	catcher.NewWhen(t.selectTests.TaskID == "", "task ID is required")
	catcher.NewWhen(t.selectTests.TaskName == "", "task name is required")
	catcher.NewWhen(len(t.selectTests.Tests) == 0, "tests array must not be empty")
	return catcher.Resolve()
}

func (t *selectTestsHandler) Run(ctx context.Context) gimlet.Responder {
	// TODO (DEVPROD-11879): remove this log once test ROI MVP no longer needs
	// this to debug inputs.
	grip.Debug(message.Fields{
		"message": "received test selection request",
		"route":   "/rest/v2/select/tests",
		"request": t.selectTests,
		"ticket":  "DEVPROD-11629",
	})

	if url := t.env.Settings().TestSelection.URL; url != "" {
		grip.Info(message.Fields{
			"message": "kim: using real test selection service",
			"url":     url,
		})
		// kim: NOTE: see more info: https://www.speakeasy.com/post/openapi-servers#servers-in-oas-3x---controlled-flexibility
		httpClient := utility.GetHTTPClient()
		defer utility.PutHTTPClient(httpClient)
		conf := testselection.NewConfiguration()
		conf.HTTPClient = httpClient
		// conf.Host = "TEST_SELECTION_URL_WITHOUT_HTTPS_GOES_HERE"
		// conf.Scheme = "https"
		conf.Servers = testselection.ServerConfigurations{
			testselection.ServerConfiguration{
				URL:         url,
				Description: "Test selection service",
			},
		}
		c := testselection.NewAPIClient(conf)
		reqBody := testselection.BodySelectTestsApiTestSelectionSelectTestsProjectIdRequesterBuildVariantNameTaskIdTaskNamePost{
			TestNames: t.selectTests.Tests,
		}
		selectedTests, resp, err := c.TestSelectionAPI.SelectTestsApiTestSelectionSelectTestsProjectIdRequesterBuildVariantNameTaskIdTaskNamePost(ctx, t.selectTests.Project, t.selectTests.Requester, t.selectTests.BuildVariant, t.selectTests.TaskID, t.selectTests.TaskName).
			BodySelectTestsApiTestSelectionSelectTestsProjectIdRequesterBuildVariantNameTaskIdTaskNamePost(reqBody).
			Execute()
		msg := message.Fields{
			"message": "kim: made request to real test selection service",
			"url":     url,
		}
		if resp != nil {
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			grip.Error(message.WrapError(err, message.Fields{
				"message": "kim: could not read raw response body",
			}))
			if len(respBody) != 0 {
				msg["tss_raw_response_body"] = string(respBody)
			}
		}
		if err != nil {
			msg["tss_error"] = err.Error()
		}
		if resp != nil {
			msg["tss_status_code"] = resp.Status
			msg["tss_content_type"] = resp.Header.Get("Content-Type")
		}
		if selectedTests != nil {
			msg["selected_tests"] = selectedTests
		}

		grip.Info(msg)

		if err != nil {
			return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "test selection service"))
		}

		rhResp := t.selectTests
		rhResp.Tests = selectedTests
		return gimlet.NewJSONResponse(rhResp)
	}

	grip.Info(message.Fields{
		"message": "kim: returning noop response without test selection service",
	})

	return gimlet.NewJSONResponse(t.selectTests)
}
