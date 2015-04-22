package cli

import (
	"10gen.com/mci/model"
	"10gen.com/mci/model/patch"
	"10gen.com/mci/util"
	"bytes"
	"encoding/json"
	"fmt"
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
	if body != nil {
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	}
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
	data := url.Values{}
	authToken, err := generateTokenParam(ac.User, ac.APIKey)
	if err != nil {
		return err
	}
	data.Set("id_token", authToken)
	data.Set("patchId", patchId)
	data.Set("action", action)
	resp, err := ac.post(fmt.Sprintf("patches/%s", patchId), bytes.NewBufferString(data.Encode()))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return NewAPIError(resp)
	}
	return nil
}

func (ac *APIClient) CancelPatch(patchId string) error {
	return ac.modifyExisting(patchId, "cancel")
}

func (ac *APIClient) FinalizePatch(patchId string) error {
	return ac.modifyExisting(patchId, "finalize")
}

// GetPatches requests a list of the user's patches from the API and returns them as a list
func (ac *APIClient) GetPatches() ([]patch.Patch, error) {
	data := url.Values{}
	authToken, err := generateTokenParam(ac.User, ac.APIKey)
	if err != nil {
		return nil, err
	}
	data.Set("id_token", authToken)
	resp, err := ac.get(fmt.Sprintf("patches/mine?%v", data.Encode()), nil)
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
	data := url.Values{}
	authToken, err := generateTokenParam(ac.User, ac.APIKey)
	if err != nil {
		return err
	}
	data.Set("id_token", authToken)
	data.Set("module", module)
	resp, err := ac.delete(fmt.Sprintf("patches/%s/modules", patchId), bytes.NewBufferString(data.Encode()))
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
	data := url.Values{}
	authToken, err := generateTokenParam(ac.User, ac.APIKey)
	if err != nil {
		return err
	}
	data.Set("id_token", authToken)
	data.Set("module", module)
	data.Set("patch", patch)
	data.Set("githash", base)
	resp, err := ac.post(fmt.Sprintf("patches/%s/modules", patchId), bytes.NewBufferString(data.Encode()))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return NewAPIError(resp)
	}
	return nil
}

// PutPatch submits a new patch for the given project to the API server. If successful, returns
// the patch object itself.
func (ac *APIClient) PutPatch(incomingPatch patchSubmission) (*patch.Patch, error) {
	authToken, err := generateTokenParam(ac.User, ac.APIKey)
	if err != nil {
		return nil, err
	}
	data := url.Values{}
	data.Set("id_token", authToken)
	data.Set("desc", incomingPatch.description)
	data.Set("project", incomingPatch.projectId)
	data.Set("patch", incomingPatch.patchData)
	data.Set("githash", incomingPatch.base)
	if incomingPatch.finalize {
		data.Set("buildvariants", incomingPatch.variants)
		data.Set("finalize", "true")
	} else {
		data.Set("buildvariants", "all")
	}
	resp, err := ac.put("patches/", bytes.NewBufferString(data.Encode()))
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
