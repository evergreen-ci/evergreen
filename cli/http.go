package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

// APIClient manages requests to the API server endpoints, and unmarshaling the results into
// usable structures.
type APIClient struct {
	APIRoot    string
	httpClient http.Client
	User       string
	APIKey     string
}

// APIError is an implementation of error for reporting unexpected results from API calls.
type APIError struct {
	body   string
	status string
}

func (ae APIError) Error() string {
	return fmt.Sprintf("Unexpected reply from server (%v): %v", ae.status, ae.body)
}

// NewAPIError creates an APIError by reading the body of the response and its status code.
func NewAPIError(resp *http.Response) APIError {
	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body) // ignore error, request has already failed anyway
	bodyStr := string(bodyBytes)
	return APIError{bodyStr, resp.Status}
}

// doReq performs a request of the given method type against path.
// If body is not nil, also includes it as a request body as url-encoded data with the
// appropriate header
func (ac *APIClient) doReq(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, fmt.Sprintf("%s/%s", ac.APIRoot, path), body)
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
		return nil, fmt.Errorf("empty response from server")
	}
	return resp, nil
}

func (ac *APIClient) get(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq("GET", path, body)
}

func (ac *APIClient) delete(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq("DELETE", path, body)
}

func (ac *APIClient) put(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq("PUT", path, body)
}

func (ac *APIClient) post(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq("POST", path, body)
}

func (ac *APIClient) modifyExisting(patchId, action string) error {
	data := struct {
		PatchId string `json:"patch_id"`
		Action  string `json:"action"`
	}{patchId, action}

	rPipe, wPipe := io.Pipe()
	encoder := json.NewEncoder(wPipe)
	go func() {
		encoder.Encode(data)
		wPipe.Close()
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
func (ac *APIClient) ValidateLocalConfig(data []byte) ([]validator.ValidationError, error) {
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

func (ac *APIClient) CancelPatch(patchId string) error {
	return ac.modifyExisting(patchId, "cancel")
}

func (ac *APIClient) FinalizePatch(patchId string) error {
	return ac.modifyExisting(patchId, "finalize")
}

// GetPatches requests a list of the user's patches from the API and returns them as a list
func (ac *APIClient) GetPatches() ([]patch.Patch, error) {
	resp, err := ac.get("patches/mine", nil)
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
func (ac *APIClient) GetProjectRef(projectId string) (*model.ProjectRef, error) {
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

// generateTokenParam constructs the authentication token to be sent with API request in the format
// expected by the server.
func generateTokenParam(user, key string) (string, error) {
	authData := struct {
		Name   string `json:"auth_user"`
		APIKey string `json:"api_key"`
	}{user, key}
	b, err := json.Marshal(authData)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DeletePatchModule makes a request to the API server to delete the given module from a patch
func (ac *APIClient) DeletePatchModule(patchId, module string) error {
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
func (ac *APIClient) UpdatePatchModule(patchId, module, patch, base string) error {
	data := struct {
		Module  string `json:"module"`
		Patch   string `json:"patch"`
		Githash string `json:"githash"`
	}{module, patch, base}

	rPipe, wPipe := io.Pipe()
	encoder := json.NewEncoder(wPipe)
	go func() {
		encoder.Encode(data)
		wPipe.Close()
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

func (ac *APIClient) ListProjects() ([]model.ProjectRef, error) {
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

// PutPatch submits a new patch for the given project to the API server. If successful, returns
// the patch object itself.
func (ac *APIClient) PutPatch(incomingPatch patchSubmission) (*patch.Patch, error) {
	data := struct {
		Description string `json:"desc"`
		Project     string `json:"project"`
		Patch       string `json:"patch"`
		Githash     string `json:"githash"`
		Variants    string `json:"buildvariants"`
		Finalize    bool   `json:"finalize"`
	}{
		incomingPatch.description,
		incomingPatch.projectId,
		incomingPatch.patchData,
		incomingPatch.base,
		"all",
		false,
	}

	if incomingPatch.finalize {
		data.Variants = incomingPatch.variants
		data.Finalize = true
	}

	rPipe, wPipe := io.Pipe()
	encoder := json.NewEncoder(wPipe)
	go func() {
		encoder.Encode(data)
		wPipe.Close()
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
func (ac *APIClient) CheckUpdates() (*evergreen.ClientConfig, error) {
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
