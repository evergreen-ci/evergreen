package operations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/client"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// legacyClient manages requests to the API server endpoints, and unmarshaling the results into
// usable structures.
type legacyClient struct {
	APIRoot            string
	httpClient         http.Client
	User               string
	APIKey             string
	APIRootV2          string
	UIRoot             string
	stagingEnvironment string
}

// APIError is an implementation of error for reporting unexpected results from API calls.
type APIError struct {
	body   string
	status string
	code   int
}

func (ae APIError) Error() string {
	return fmt.Sprintf("Unexpected reply from server (%s): %s", ae.status, ae.body)
}

// NewAPIError creates an APIError by reading the body of the response and its status code.
func NewAPIError(resp *http.Response) APIError {
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body) // ignore error, request has already failed anyway
	bodyStr := string(bodyBytes)
	return APIError{bodyStr, resp.Status, resp.StatusCode}
}

func NewAuthError(resp *http.Response) APIError {
	apiError := NewAPIError(resp)
	apiError.body = fmt.Sprintf("%s (%s)", apiError.body, client.AuthError)
	return apiError
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

	req.Header.Add(evergreen.APIKeyHeader, ac.APIKey)
	req.Header.Add(evergreen.APIUserHeader, ac.User)
	if ac.stagingEnvironment != "" {
		req.Header.Add(evergreen.EnvironmentHeader, ac.stagingEnvironment)
	}

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
	return ac.doReq(http.MethodGet, path, 1, body)
}

func (ac *legacyClient) get2(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq(http.MethodGet, path, 2, body)
}

func (ac *legacyClient) delete(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq(http.MethodDelete, path, 1, body)
}

func (ac *legacyClient) put(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq(http.MethodPut, path, 1, body)
}

func (ac *legacyClient) post(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq(http.MethodPost, path, 1, body)
}

func (ac *legacyClient) post2(path string, body io.Reader) (*http.Response, error) {
	return ac.doReq(http.MethodPost, path, 2, body)
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
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return NewAPIError(resp)
	}
	return nil
}

// ValidateLocalConfig validates the local project config with the server
func (ac *legacyClient) ValidateLocalConfig(data []byte, quiet, includeLong bool, projectID string) (validator.ValidationErrors, error) {
	input := validator.ValidationInput{
		ProjectYaml: data,
		Quiet:       quiet,
		IncludeLong: includeLong,
		ProjectID:   projectID,
	}
	rPipe, wPipe := io.Pipe()
	encoder := json.NewEncoder(wPipe)
	go func() {
		grip.Warning(encoder.Encode(input))
		grip.Warning(wPipe.Close())
	}()
	resp, err := ac.post("validate", rPipe)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		errors := validator.ValidationErrors{}
		err = utility.ReadJSON(resp.Body, &errors)
		if err != nil {
			return nil, NewAPIError(resp)
		}
		return errors, nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
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
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	patches := []patch.Patch{}
	if err := utility.ReadJSON(resp.Body, &patches); err != nil {
		return nil, err
	}
	return patches, nil
}

// GetRestPatch gets a patch from the server given a patch id and returns it as a RestPatch.
func (ac *legacyClient) GetRestPatch(patchId string) (*service.RestPatch, error) {
	resp, err := ac.get(fmt.Sprintf("patches/%v", patchId), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	result := &service.RestPatch{}
	if err := utility.ReadJSON(resp.Body, result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetPatch gets a patch from the server given a patch id and returns it as a Patch.
func (ac *legacyClient) GetPatch(patchId string) (*patch.Patch, error) {
	resp, err := ac.get2(fmt.Sprintf("patches/%v", patchId), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	apiModel := &restModel.APIPatch{}
	if err = utility.ReadJSON(resp.Body, apiModel); err != nil {
		return nil, err
	}
	res, err := apiModel.ToService()
	if err != nil {
		return nil, errors.Wrapf(err, "converting patch to service model")
	}
	return &res, nil
}

// GetProjectRef requests project details from the API server for a given project ID.
func (ac *legacyClient) GetProjectRef(projectId string) (*model.ProjectRef, error) {
	resp, err := ac.get(fmt.Sprintf("ref/%s", projectId), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	ref := &model.ProjectRef{}
	if err := utility.ReadJSON(resp.Body, ref); err != nil {
		return nil, err
	}
	return ref, nil
}

// GetPatchedConfig takes in patch id and returns the patched project config.
func (ac *legacyClient) GetPatchedConfig(patchId string) (*model.Project, error) {
	resp, err := ac.get(fmt.Sprintf("patches/%s/config", patchId), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	ref := &model.Project{}
	yamlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if _, err := model.LoadProjectInto(resp.Request.Context(), yamlBytes, nil, "", ref); err != nil {
		return nil, err
	}
	return ref, nil
}

// GetConfig fetches the config yaml from the API server for a given project ID.
func (ac *legacyClient) GetConfig(versionId string) ([]byte, error) {
	path := fmt.Sprintf("versions/%s/config", versionId)
	resp, err := ac.get(path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}
	return respBytes, nil

}

// GetProject fetches the project details from the API server for a given project ID.
func (ac *legacyClient) GetProject(versionId string) (*model.Project, error) {
	resp, err := ac.get(fmt.Sprintf("versions/%s/parser_project", versionId), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}

	return model.GetProjectFromBSON(respBytes)
}

// GetLastGreen returns the most recent successful version for the given project and variants.
func (ac *legacyClient) GetLastGreen(project string, variants []string) (*model.Version, error) {
	qs := []string{}
	for _, v := range variants {
		qs = append(qs, url.QueryEscape(v))
	}
	q := strings.Join(qs, "&")
	resp, err := ac.get(fmt.Sprintf("projects/%s/last_green?%s", project, q), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	v := &model.Version{}
	if err := utility.ReadJSON(resp.Body, v); err != nil {
		return nil, err
	}
	return v, nil
}

// DeletePatchModule makes a request to the API server to delete the given module from a patch
func (ac *legacyClient) DeletePatchModule(patchId, module string) error {
	resp, err := ac.delete(fmt.Sprintf("patches/%s/modules?module=%s", patchId, url.QueryEscape(module)), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return NewAPIError(resp)
	}
	return nil
}

type UpdatePatchModuleParams struct {
	patchID string
	module  string
	patch   string
	base    string
	message string
}

// UpdatePatchModule makes a request to the API server to set a module patch on the given patch ID.
func (ac *legacyClient) UpdatePatchModule(params UpdatePatchModuleParams) error {
	// Characters in a string without a utf-8 representation are shoehorned into the � replacement character
	// when marshalled into JSON.
	// Because marshalling a byte slice to JSON will base64 encode it, the patch will be sent over the wire in base64
	// and non utf-8 characters will be preserved.
	data := struct {
		Module     string `json:"module"`
		PatchBytes []byte `json:"patch_bytes"`
		Githash    string `json:"githash"`
		Message    string `json:"message"`
	}{params.module, []byte(params.patch), params.base, params.message}

	rPipe, wPipe := io.Pipe()
	encoder := json.NewEncoder(wPipe)
	go func() {
		grip.Warning(encoder.Encode(data))
		grip.Warning(wPipe.Close())
	}()
	defer rPipe.Close()

	resp, err := ac.post(fmt.Sprintf("patches/%s/modules", params.patchID), rPipe)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return NewAuthError(resp)
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
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	projs := []model.ProjectRef{}
	if err := utility.ReadJSON(resp.Body, &projs); err != nil {
		return nil, err
	}
	return projs, nil
}

func (ac *legacyClient) ListTasks(project string) ([]model.ProjectTask, error) {
	resp, err := ac.get(fmt.Sprintf("tasks/%s", project), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	tasks := []model.ProjectTask{}
	if err := utility.ReadJSON(resp.Body, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (ac *legacyClient) ListVariants(project string) ([]model.BuildVariant, error) {
	resp, err := ac.get(fmt.Sprintf("variants/%s", project), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}
	variants := []model.BuildVariant{}
	if err := utility.ReadJSON(resp.Body, &variants); err != nil {
		return nil, err
	}
	return variants, nil
}

func (ac *legacyClient) ListDistros() ([]distro.Distro, error) {
	resp, err := ac.get2("distros", nil)
	if err != nil {
		return nil, errors.Wrap(err, "making request to get distros")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Wrap(NewAPIError(resp), "received non-OK status from API server")
	}
	distros := []distro.Distro{}
	if err := utility.ReadJSON(resp.Body, &distros); err != nil {
		return nil, errors.Wrap(err, "reading JSON from response body")
	}
	return distros, nil
}

// PutPatch submits a new patch for the given project to the API server. If successful, returns
// the patch object itself.
func (ac *legacyClient) PutPatch(incomingPatch patchSubmission) (*patch.Patch, error) {
	// Characters in a string without a utf-8 representation are shoehorned into the � replacement character
	// when marshalled into JSON.
	// Because marshalling a byte slice to JSON will base64 encode it, the patch will be sent over the wire in base64
	// and non utf-8 characters will be preserved.
	data := struct {
		Description         string                     `json:"desc"`
		Project             string                     `json:"project"`
		Path                string                     `json:"path"`
		PatchBytes          []byte                     `json:"patch_bytes"`
		Githash             string                     `json:"githash"`
		Alias               string                     `json:"alias"`
		Variants            []string                   `json:"buildvariants_new"`
		Tasks               []string                   `json:"tasks"`
		RegexVariants       []string                   `json:"regex_buildvariants"`
		RegexTasks          []string                   `json:"regex_tasks"`
		SyncTasks           []string                   `json:"sync_tasks"`
		SyncBuildVariants   []string                   `json:"sync_build_variants"`
		SyncStatuses        []string                   `json:"sync_statuses"`
		SyncTimeout         time.Duration              `json:"sync_timeout"`
		Finalize            bool                       `json:"finalize"`
		BackportInfo        patch.BackportInfo         `json:"backport_info"`
		TriggerAliases      []string                   `json:"trigger_aliases"`
		Parameters          []patch.Parameter          `json:"parameters"`
		GitMetadata         patch.GitMetadata          `json:"git_metadata"`
		RepeatDefinition    bool                       `json:"reuse_definition"`
		RepeatFailed        bool                       `json:"repeat_failed"`
		RepeatPatchId       string                     `json:"repeat_patch_id"`
		GithubAuthor        string                     `json:"github_author"`
		PatchAuthor         string                     `json:"patch_author"`
		LocalModuleIncludes []patch.LocalModuleInclude `json:"local_module_includes"`
	}{
		Description:         incomingPatch.description,
		Project:             incomingPatch.projectName,
		Path:                incomingPatch.path,
		PatchBytes:          []byte(incomingPatch.patchData),
		Githash:             incomingPatch.base,
		Alias:               incomingPatch.alias,
		Variants:            incomingPatch.variants,
		Tasks:               incomingPatch.tasks,
		RegexVariants:       incomingPatch.regexVariants,
		RegexTasks:          incomingPatch.regexTasks,
		SyncBuildVariants:   incomingPatch.syncBuildVariants,
		SyncTasks:           incomingPatch.syncTasks,
		SyncStatuses:        incomingPatch.syncStatuses,
		SyncTimeout:         incomingPatch.syncTimeout,
		Finalize:            incomingPatch.finalize,
		BackportInfo:        incomingPatch.backportOf,
		TriggerAliases:      incomingPatch.triggerAliases,
		Parameters:          incomingPatch.parameters,
		GitMetadata:         incomingPatch.gitMetadata,
		RepeatDefinition:    incomingPatch.repeatDefinition,
		RepeatFailed:        incomingPatch.repeatFailed,
		RepeatPatchId:       incomingPatch.repeatPatchId,
		GithubAuthor:        incomingPatch.githubAuthor,
		PatchAuthor:         incomingPatch.patchAuthor,
		LocalModuleIncludes: incomingPatch.localModuleIncludes,
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

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, NewAPIError(resp)
	}

	reply := struct {
		Patch *patch.Patch `json:"patch"`
	}{}

	if err := utility.ReadJSON(resp.Body, &reply); err != nil {
		return nil, err
	}

	return reply.Patch, nil
}

func (ac *legacyClient) GetTask(taskId string) (*service.RestTask, error) {
	resp, err := ac.get("tasks/"+taskId, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}

	reply := service.RestTask{}
	if err := utility.ReadJSON(resp.Body, &reply); err != nil {
		return nil, err
	}
	return &reply, nil
}

// GetPatchModules retrieves a list of modules available for a given patch, along with the project identifier.
func (ac *legacyClient) GetPatchModules(patchId, projectId string) ([]string, string, error) {
	var out []string

	resp, err := ac.get(fmt.Sprintf("patches/%s/%s/modules", patchId, projectId), nil)
	if err != nil {
		return out, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, "", NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return out, "", NewAPIError(resp)
	}

	data := struct {
		Project string   `json:"project"`
		Modules []string `json:"modules"`
	}{}

	err = utility.ReadJSON(resp.Body, &data)
	if err != nil {
		return out, "", err
	}

	return data.Modules, data.Project, nil
}

// GetRecentVersions retrieves a list of recent versions for a project,
// regardless of their success
func (ac *legacyClient) GetRecentVersions(projectID string) ([]string, error) {
	resp, err := ac.get(fmt.Sprintf("projects/%s/versions", projectID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAPIError(resp)
	}

	v := struct {
		Versions []struct {
			Id string `json:"version_id"`
		} `json:"versions"`
	}{}

	err = utility.ReadJSON(resp.Body, &v)
	if err != nil {
		return nil, err
	}

	out := []string{}
	for _, v := range v.Versions {
		out = append(out, v.Id)
	}

	return out, nil
}

func (ac *legacyClient) UpdateRole(role *gimlet.Role) error {
	if role == nil {
		return errors.New("no role to update")
	}
	roleJSON, err := json.Marshal(role)
	if err != nil {
		return errors.Wrap(err, "marshalling role data")
	}
	resp, err := ac.post2("roles", bytes.NewBuffer(roleJSON))
	if err != nil {
		return errors.Wrap(err, "making request to update role")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return NewAuthError(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return NewAPIError(resp)
	}
	return nil
}
