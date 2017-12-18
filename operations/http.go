package operations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// legacyClient manages requests to the API server endpoints, and unmarshaling the results into
// usable structures.
type legacyClient struct {
	APIRoot    string
	httpClient http.Client
	User       string
	APIKey     string
	APIRootV2  string
	UIRoot     string
}

// APIError is an implementation of error for reporting unexpected results from API calls.
type APIError struct {
	body   string
	status string
	code   int
}

func (ae APIError) Error() string {
	return fmt.Sprintf("Unexpected reply from server (%v): %v", ae.status, ae.body)
}

// NewAPIError creates an APIError by reading the body of the response and its status code.
func NewAPIError(resp *http.Response) APIError {
	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body) // ignore error, request has already failed anyway
	bodyStr := string(bodyBytes)
	return APIError{bodyStr, resp.Status, resp.StatusCode}
}

// doReq performs a request of the given method type against path.
// If body is not nil, also includes it as a request body as url-encoded data with the
// appropriate header
func (ac *legacyClient) doReq(method, path string, apiVersion int, body io.Reader) (*http.Response, error) {
	var req *http.Request
	var err error

	if apiVersion == 1 {
		req, err = http.NewRequest(method, fmt.Sprintf("%s/%s", ac.APIRoot, path), body)
	} else if apiVersion == 2 {
		req, err = http.NewRequest(method, fmt.Sprintf("%s/%s", ac.APIRootV2, path), body)
	} else if apiVersion == -1 {
		req, err = http.NewRequest(method, fmt.Sprintf("%s/%s", ac.UIRoot, path), body)
	} else {
		return nil, errors.Errorf("invalid apiVersion")
	}
	if err != nil {
		return nil, err
	}

	req.Header.Add("Api-Key", ac.APIKey)
	req.Header.Add("Api-User", ac.User)
	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("empty response from server")
	}
	return resp, nil
}

func (ac *legacyClient) get(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq("GET", path, 1, body)
}

func (ac *legacyClient) get2(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq("GET", path, 2, body)
}

func (ac *legacyClient) delete(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq("DELETE", path, 1, body)
}

func (ac *legacyClient) put(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq("PUT", path, 1, body)
}

func (ac *legacyClient) post(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq("POST", path, 1, body)
}

func (ac *legacyClient) modifyExisting(patchId, action string) error {
	data := struct {
		PatchId string `json:"patch_id"`
		Action  string `json:"action"`
	}{patchId, action}

	rPipe, wPipe := io.Pipe()
	encoder := json.NewEncoder(wPipe)
	go func() {
		grip.Warning(encoder.Encode(data))
		grip.Warning(wPipe.Close())
	}()
	defer rPipe.Close()

	resp, err := ac.post(fmt.Sprintf("patches/%s", patchId), rPipe)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return NewAPIError(resp)
	}
	return nil
}

// ValidateLocalConfig validates the local project config with the server
func (ac *legacyClient) ValidateLocalConfig(data []byte) ([]validator.ValidationError, error) {
	resp, err := ac.post("validate", bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusBadRequest {
		errors := []validator.ValidationError{}
		err = util.ReadJSONInto(resp.Body, &errors)
		if err != nil {
			return nil, NewAPIError(resp)
		}
		return errors, nil
	} else if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	return nil, nil
}

func (ac *legacyClient) CancelPatch(patchId string) error {
	return ac.modifyExisting(patchId, "cancel")
}

func (ac *legacyClient) FinalizePatch(patchId string) error {
	return ac.modifyExisting(patchId, "finalize")
}

// GetPatches requests a list of the user's patches from the API and returns them as a list
func (ac *legacyClient) GetPatches(n int) ([]patch.Patch, error) {
	resp, err := ac.get(fmt.Sprintf("patches/mine?n=%v", n), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	patches := []patch.Patch{}
	if err := util.ReadJSONInto(resp.Body, &patches); err != nil {
		return nil, err
	}
	return patches, nil
}

// GetProjectRef requests project details from the API server for a given project ID.
func (ac *legacyClient) GetProjectRef(projectId string) (*model.ProjectRef, error) {
	resp, err := ac.get(fmt.Sprintf("/ref/%s", projectId), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	ref := &model.ProjectRef{}
	if err := util.ReadJSONInto(resp.Body, ref); err != nil {
		return nil, err
	}
	return ref, nil
}

// GetPatch gets a patch from the server given a patch id.
func (ac *legacyClient) GetPatch(patchId string) (*service.RestPatch, error) {
	resp, err := ac.get(fmt.Sprintf("patches/%v", patchId), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	ref := &service.RestPatch{}
	if err := util.ReadJSONInto(resp.Body, ref); err != nil {
		return nil, err
	}
	return ref, nil
}

// GetPatchedConfig takes in patch id and returns the patched project config.
func (ac *legacyClient) GetPatchedConfig(patchId string) (*model.Project, error) {
	resp, err := ac.get(fmt.Sprintf("patches/%v/config", patchId), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	ref := &model.Project{}
	if err := util.ReadYAMLInto(resp.Body, ref); err != nil {
		return nil, err
	}
	return ref, nil
}

// GetVersionConfig fetches the config requests project details from the API server for a given project ID.
func (ac *legacyClient) GetConfig(versionId string) (*model.Project, error) {
	resp, err := ac.get(fmt.Sprintf("versions/%v/config", versionId), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	ref := &model.Project{}
	if err := util.ReadYAMLInto(resp.Body, ref); err != nil {
		return nil, err
	}
	return ref, nil
}

// GetLastGreen returns the most recent successful version for the given project and variants.
func (ac *legacyClient) GetLastGreen(project string, variants []string) (*version.Version, error) {
	qs := []string{}
	for _, v := range variants {
		qs = append(qs, url.QueryEscape(v))
	}
	q := strings.Join(qs, "&")
	resp, err := ac.get(fmt.Sprintf("projects/%v/last_green?%v", project, q), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	v := &version.Version{}
	if err := util.ReadJSONInto(resp.Body, v); err != nil {
		return nil, err
	}
	return v, nil
}

// DeletePatchModule makes a request to the API server to delete the given module from a patch
func (ac *legacyClient) DeletePatchModule(patchId, module string) error {
	resp, err := ac.delete(fmt.Sprintf("patches/%s/modules?module=%v", patchId, url.QueryEscape(module)), nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return NewAPIError(resp)
	}
	return nil
}

// UpdatePatchModule makes a request to the API server to set a module patch on the given patch ID.
func (ac *legacyClient) UpdatePatchModule(patchId, module, patch, base string) error {
	data := struct {
		Module  string `json:"module"`
		Patch   string `json:"patch"`
		Githash string `json:"githash"`
	}{module, patch, base}

	rPipe, wPipe := io.Pipe()
	encoder := json.NewEncoder(wPipe)
	go func() {
		grip.Warning(encoder.Encode(data))
		grip.Warning(wPipe.Close())
	}()
	defer rPipe.Close()

	resp, err := ac.post(fmt.Sprintf("patches/%s/modules", patchId), rPipe)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return NewAPIError(resp)
	}
	return nil
}

func (ac *legacyClient) ListProjects() ([]model.ProjectRef, error) {
	resp, err := ac.get("projects", nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	projs := []model.ProjectRef{}
	if err := util.ReadJSONInto(resp.Body, &projs); err != nil {
		return nil, err
	}
	return projs, nil
}

func (ac *legacyClient) ListTasks(project string) ([]model.ProjectTask, error) {
	resp, err := ac.get(fmt.Sprintf("tasks/%v", project), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	tasks := []model.ProjectTask{}
	if err := util.ReadJSONInto(resp.Body, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (ac *legacyClient) ListVariants(project string) ([]model.BuildVariant, error) {
	resp, err := ac.get(fmt.Sprintf("variants/%v", project), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	variants := []model.BuildVariant{}
	if err := util.ReadJSONInto(resp.Body, &variants); err != nil {
		return nil, err
	}
	return variants, nil
}

func (ac *legacyClient) ListDistros() ([]distro.Distro, error) {
	resp, err := ac.get2("distros", nil)
	if err != nil {
		return nil, errors.Wrap(err, "problem querying api server")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Wrap(NewAPIError(resp), "bad status from api server")
	}
	distros := []distro.Distro{}
	if err := util.ReadJSONInto(resp.Body, &distros); err != nil {
		return nil, errors.Wrap(err, "error reading json")
	}
	return distros, nil
}

// PutPatch submits a new patch for the given project to the API server. If successful, returns
// the patch object itself.
func (ac *legacyClient) PutPatch(incomingPatch patchSubmission) (*patch.Patch, error) {
	data := struct {
		Description string   `json:"desc"`
		Project     string   `json:"project"`
		Patch       string   `json:"patch"`
		Githash     string   `json:"githash"`
		Variants    string   `json:"buildvariants"` //TODO make this an array
		Tasks       []string `json:"tasks"`
		Finalize    bool     `json:"finalize"`
	}{
		incomingPatch.description,
		incomingPatch.projectId,
		incomingPatch.patchData,
		incomingPatch.base,
		incomingPatch.variants,
		incomingPatch.tasks,
		incomingPatch.finalize,
	}

	rPipe, wPipe := io.Pipe()
	encoder := json.NewEncoder(wPipe)
	go func() {
		grip.Warning(encoder.Encode(data))
		grip.Warning(wPipe.Close())
	}()
	defer rPipe.Close()

	resp, err := ac.put("patches/", rPipe)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, NewAPIError(resp)
	}

	reply := struct {
		Patch *patch.Patch `json:"patch"`
	}{}

	if err := util.ReadJSONInto(resp.Body, &reply); err != nil {
		return nil, err
	}

	return reply.Patch, nil
}

// CheckUpdates fetches information about available updates to client binaries from the server.
func (ac *legacyClient) CheckUpdates() (*evergreen.ClientConfig, error) {
	resp, err := ac.get("update", nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}

	reply := evergreen.ClientConfig{}
	if err := util.ReadJSONInto(resp.Body, &reply); err != nil {
		return nil, err
	}
	return &reply, nil
}

func (ac *legacyClient) GetTask(taskId string) (*service.RestTask, error) {
	resp, err := ac.get("tasks/"+taskId, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}

	reply := service.RestTask{}
	if err := util.ReadJSONInto(resp.Body, &reply); err != nil {
		return nil, err
	}
	return &reply, nil
}

// GetHostUtilizationStats takes in an integer granularity, which is in seconds, and the number of days back and makes a
// REST API call to get host utilization statistics.
func (ac *legacyClient) GetHostUtilizationStats(granularity, daysBack int, csv bool) (io.ReadCloser, error) {
	resp, err := ac.get(fmt.Sprintf("scheduler/host_utilization?granularity=%v&numberDays=%v&csv=%v",
		granularity, daysBack, csv), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("not found")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}

	return resp.Body, nil
}

// GetAverageSchedulerStats takes in an integer granularity, which is in seconds, the number of days back, and a distro id
// and makes a REST API call to get host utilization statistics.
func (ac *legacyClient) GetAverageSchedulerStats(granularity, daysBack int, distroId string, csv bool) (io.ReadCloser, error) {
	resp, err := ac.get(fmt.Sprintf("scheduler/distro/%v/stats?granularity=%v&numberDays=%v&csv=%v",
		distroId, granularity, daysBack, csv), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("not found")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}

	return resp.Body, nil
}

// GetOptimalMakespan takes in an integer granularity, which is in seconds, and the number of days back and makes a
// REST API call to get the optimal and actual makespan for builds going back however many days.
func (ac *legacyClient) GetOptimalMakespans(numberBuilds int, csv bool) (io.ReadCloser, error) {
	resp, err := ac.get(fmt.Sprintf("scheduler/makespans?number=%v&csv=%v", numberBuilds, csv), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("not found")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}

	return resp.Body, nil
}

// GetTestHistory takes in a project identifier, the url query parameter string, and a csv flag and
// returns the body of the response of the test_history api endpoint.
func (ac *legacyClient) GetTestHistory(project, queryParams string, isCSV bool) (io.ReadCloser, error) {
	if isCSV {
		queryParams += "&csv=true"
	}
	resp, err := ac.get(fmt.Sprintf("projects/%v/test_history?%v", project, queryParams), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("not found")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}

	return resp.Body, nil
}

// GetPatchModules retrieves a list of modules available for a given patch.
func (ac *legacyClient) GetPatchModules(patchId, projectId string) ([]string, error) {
	var out []string

	resp, err := ac.get(fmt.Sprintf("patches/%s/%s/modules", patchId, projectId), nil)
	if err != nil {
		return out, err
	}

	if resp.StatusCode != http.StatusOK {
		return out, NewAPIError(resp)
	}

	data := struct {
		Project string   `json:"project"`
		Modules []string `json:"modules"`
	}{}

	err = util.ReadJSONInto(resp.Body, &data)
	if err != nil {
		return out, err
	}
	out = data.Modules

	return out, nil
}
