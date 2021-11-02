package data

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

func validatePatchID(patchId string) error {
	if !mgobson.IsObjectIdHex(patchId) {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("id '%s' is not a valid patch id", patchId),
		}
	}

	return nil
}

// DBPatchConnector is a struct that implements the Patch related methods
// from the Connector through interactions with the backing database.
type DBPatchConnector struct{}

// FindPatchesByProject uses the service layer's patches type to query the backing database for
// the patches.
func (pc *DBPatchConnector) FindPatchesByProject(projectId string, ts time.Time, limit int) ([]restModel.APIPatch, error) {
	apiPatches := []restModel.APIPatch{}
	id, err := model.GetIdForProject(projectId)
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching project with id %s", projectId)
	}
	patches, err := patch.Find(patch.PatchesByProject(id, ts, limit))
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching patches for project %s", id)
	}
	for _, p := range patches {
		apiPatch := restModel.APIPatch{}
		err = apiPatch.BuildFromService(p)
		if err != nil {
			return nil, errors.Wrap(err, "problem converting patch")
		}
		apiPatches = append(apiPatches, apiPatch)
	}

	return apiPatches, nil
}

// FindPatchById queries the backing database for the patch matching patchId.
func (pc *DBPatchConnector) FindPatchById(patchId string) (*restModel.APIPatch, error) {
	if err := validatePatchID(patchId); err != nil {
		return nil, errors.WithStack(err)
	}

	p, err := patch.FindOneId(patchId)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch with id %s not found", patchId),
		}
	}

	apiPatch := restModel.APIPatch{}
	err = apiPatch.BuildFromService(*p)
	if err != nil {
		return nil, errors.Wrap(err, "problem converting patch")
	}

	return &apiPatch, nil
}

// GetChildPatchIds queries the backing database for the child patch ids
func (pc *DBPatchConnector) GetChildPatchIds(patchId string) ([]string, error) {
	if err := validatePatchID(patchId); err != nil {
		return nil, errors.WithStack(err)
	}
	p, err := patch.FindOneId(patchId)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch with id %s not found", patchId),
		}
	}

	return p.Triggers.ChildPatches, nil
}

func (pc *DBPatchConnector) FindPatchesByIds(patchIds []string) ([]restModel.APIPatch, error) {
	patchObjectIDs := []mgobson.ObjectId{}
	for _, patchID := range patchIds {
		if err := validatePatchID(patchID); err != nil {
			return nil, errors.Wrap(err, "problem validating patchId")
		}
		patchObjectIDs = append(patchObjectIDs, mgobson.ObjectIdHex(patchID))
	}

	p, err := patch.Find(patch.ByIds(patchObjectIDs))
	if err != nil {
		return nil, errors.Wrap(err, "error finding patches")
	}
	if p == nil {
		return nil, errors.New("patches not found")
	}

	apiPatches := []restModel.APIPatch{}
	for _, patch := range p {
		apiPatch := restModel.APIPatch{}
		err = apiPatch.BuildFromService(patch)
		if err != nil {
			return nil, errors.Wrap(err, "problem converting patch")
		}
		apiPatches = append(apiPatches, apiPatch)
	}
	return apiPatches, nil

}

// AbortPatch uses the service level CancelPatch method to abort a single patch
// with matching Id.
func (pc *DBPatchConnector) AbortPatch(patchId string, user string) error {
	if err := validatePatchID(patchId); err != nil {
		return errors.WithStack(err)
	}

	p, err := patch.FindOne(patch.ById(mgobson.ObjectIdHex(patchId)))
	if err != nil {
		return err
	}
	if p == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch with id %s not found", patchId),
		}
	}
	return model.CancelPatch(p, task.AbortInfo{User: user})
}

// SetPatchPriority attempts to set the priority on the corresponding version.
// Will not error if no version exists.
func (pc *DBPatchConnector) SetPatchPriority(patchId string, priority int64, caller string) error {
	return model.SetVersionPriority(patchId, priority, caller)
}

// SetPatchActivated attempts to activate the patch and create a new version (if activated is set to true)
func (pc *DBPatchConnector) SetPatchActivated(ctx context.Context, patchId string, user string, activated bool, settings *evergreen.Settings) error {
	p, err := patch.FindOne(patch.ById(mgobson.ObjectIdHex(patchId)))
	if err != nil {
		return err
	}
	err = p.SetActivation(activated)
	if err != nil {
		return err
	}
	if activated && p.Version == "" {
		requester := p.GetRequester()

		token, err := settings.GetGithubOauthToken()
		if err != nil {
			return errors.Wrap(err, "error getting github token from settings")
		}
		if _, err = model.FinalizePatch(ctx, p, requester, token); err != nil {
			return errors.Wrapf(err, "error finalizing patch '%s'", p.Id.Hex())
		}
		if requester == evergreen.PatchVersionRequester {
			grip.Info(message.Fields{
				"operation":     "patch creation",
				"message":       "finalized patch",
				"from":          "rest route",
				"patch_id":      p.Id,
				"variants":      p.BuildVariants,
				"tasks":         p.Tasks,
				"variant_tasks": p.VariantsTasks,
				"alias":         p.Alias,
			})
		}
		if p.IsGithubPRPatch() {
			job := units.NewGithubStatusUpdateJobForNewPatch(p.Id.Hex())
			q := evergreen.GetEnvironment().LocalQueue()
			if err := q.Put(ctx, job); err != nil {
				return errors.Wrap(err, "Error adding github status update job to queue")
			}
		}
	}

	return model.SetVersionActivation(patchId, activated, user)
}

func (pc *DBPatchConnector) FindPatchesByUser(user string, ts time.Time, limit int) ([]restModel.APIPatch, error) {
	patches, err := patch.Find(patch.ByUserPaginated(user, ts, limit))
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching patches for user %s", user)
	}
	apiPatches := []restModel.APIPatch{}
	for _, p := range patches {
		apiPatch := restModel.APIPatch{}
		err = apiPatch.BuildFromService(p)
		if err != nil {
			return nil, errors.Wrap(err, "problem converting patch")
		}
		apiPatches = append(apiPatches, apiPatch)
	}

	return apiPatches, nil
}

func (p *DBPatchConnector) AbortPatchesFromPullRequest(event *github.PullRequestEvent) error {
	owner, repo, err := verifyPullRequestEventForAbort(event)
	if err != nil {
		return err
	}

	err = model.AbortPatchesWithGithubPatchData(*event.PullRequest.ClosedAt, true, "",
		owner, repo, *event.Number)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "error aborting patches",
		}
	}

	return nil
}

func (p *DBPatchConnector) IsPatchEmpty(id string) (bool, error) {
	patchDoc, err := patch.FindOne(patch.ByStringId(id).WithFields(patch.PatchesKey))
	if err != nil {
		return false, errors.WithStack(err)
	}
	if patchDoc == nil {
		return false, errors.New("patch is empty")
	}
	return len(patchDoc.Patches) == 0, nil
}

func (p *DBPatchConnector) GetPatchRawPatches(patchID string) (map[string]string, error) {
	patchDoc, err := patch.FindOneId(patchID)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "can't get patch").Error(),
		}
	}
	if patchDoc == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no patch '%s' found", patchID),
		}
	}

	// set the patch status to the collective status between the parent and child patches
	if patchDoc.IsParent() {
		allStatuses := []string{patchDoc.Status}
		for _, childPatchId := range patchDoc.Triggers.ChildPatches {
			cp, err := patch.FindOneId(childPatchId)
			if err != nil {
				return nil, gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    errors.Wrap(err, "can't get child patch").Error(),
				}
			}
			if cp == nil {
				return nil, gimlet.ErrorResponse{
					StatusCode: http.StatusNotFound,
					Message:    fmt.Sprintf("child patch with id %s not found", childPatchId),
				}
			}
			allStatuses = append(allStatuses, cp.Status)
		}

		patchDoc.Status = patch.GetCollectiveStatus(allStatuses)
	}

	if err = patchDoc.FetchPatchFiles(false); err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "can't get patch contents").Error(),
		}
	}

	patchMap := make(map[string]string)
	for _, raw := range patchDoc.Patches {
		patchMap[raw.ModuleName] = raw.PatchSet.Patch
	}

	return patchMap, nil
}

// MockPatchConnector is a struct that implements the Patch related methods
// from the Connector through interactions with he backing database.
type MockPatchConnector struct {
	CachedPatches     []restModel.APIPatch
	CachedProjectRefs []restModel.APIProjectRef
	CachedAborted     map[string]string
	CachedPriority    map[string]int64
	CachedRawPatches  map[string]string
}

// FindPatchesByProject queries the cached patches splice for the matching patches.
// Assumes CachedPatches is sorted by increasing creation time.
func (hp *MockPatchConnector) FindPatchesByProject(projectId string, ts time.Time, limit int) ([]restModel.APIPatch, error) {
	patchesToReturn := []restModel.APIPatch{}
	if limit <= 0 {
		return patchesToReturn, nil
	}
	var id string
	for _, p := range hp.CachedProjectRefs {
		if *p.Id == projectId || *p.Identifier == projectId {
			id = *p.Id
		}
	}
	if id == "" {
		return nil, errors.Errorf("failed to find project '%s'", projectId)
	}
	for i := len(hp.CachedPatches) - 1; i >= 0; i-- {
		p := hp.CachedPatches[i]
		if *p.ProjectId == id && !p.CreateTime.After(ts) {
			patchesToReturn = append(patchesToReturn, p)
			if len(patchesToReturn) == limit {
				break
			}
		}
	}
	return patchesToReturn, nil
}

// FindPatchById iterates through the slice of CachedPatches to find the matching patch.
func (pc *MockPatchConnector) FindPatchById(patchId string) (*restModel.APIPatch, error) {
	for idx := range pc.CachedPatches {
		if *pc.CachedPatches[idx].Id == patchId {
			return &pc.CachedPatches[idx], nil
		}
	}
	return nil, gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("patch with id %s not found", patchId),
	}
}

// GetChildPatchIds is not implemented
func (pc *MockPatchConnector) GetChildPatchIds(patchId string) ([]string, error) {
	return nil, gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    "not implemented",
	}
}

func (pc *MockPatchConnector) FindPatchesByIds(patchIds []string) ([]restModel.APIPatch, error) {
	return nil, nil
}

// AbortPatch sets the value of patchId in CachedAborted to user.
func (pc *MockPatchConnector) AbortPatch(patchId string, user string) error {
	var foundPatch *restModel.APIPatch
	var foundIdx int
	for idx, p := range pc.CachedPatches {
		if *p.Id == patchId {
			foundPatch = &p
			foundIdx = idx
			break
		}
	}
	if foundPatch == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch with id %s not found", patchId),
		}
	}
	pc.CachedAborted[patchId] = user
	if foundPatch.Version == nil || *foundPatch.Version == "" {
		pc.CachedPatches = append(pc.CachedPatches[:foundIdx], pc.CachedPatches[foundIdx+1:]...)
	}
	return nil
}

// SetPatchPriority sets the patch priority in the CachedPriority map.
func (pc *MockPatchConnector) SetPatchPriority(patchId string, priority int64, caller string) error {
	pc.CachedPriority[patchId] = priority
	return nil
}

// SetPatchActivated sets the boolean activated field on the input patch.
func (pc *MockPatchConnector) SetPatchActivated(ctx context.Context, patchId string, user string, activated bool, settings *evergreen.Settings) error {
	p, err := pc.FindPatchById(patchId)
	if err != nil {
		return err
	}
	p.Activated = activated
	return nil
}

// FindPatchesByUser iterates through the cached patches slice to find the correct patches
func (hp *MockPatchConnector) FindPatchesByUser(user string, ts time.Time, limit int) ([]restModel.APIPatch, error) {
	patchesToReturn := []restModel.APIPatch{}
	if limit <= 0 {
		return patchesToReturn, nil
	}
	for i := len(hp.CachedPatches) - 1; i >= 0; i-- {
		p := hp.CachedPatches[i]
		if p.Author != nil && *p.Author == user && !p.CreateTime.After(ts) {
			patchesToReturn = append(patchesToReturn, p)
			if len(patchesToReturn) == limit {
				break
			}
		}
	}
	return patchesToReturn, nil
}

func (c *MockPatchConnector) AbortPatchesFromPullRequest(event *github.PullRequestEvent) error {
	_, _, err := verifyPullRequestEventForAbort(event)
	return err
}

func (c *MockPatchConnector) GetPatchRawPatches(patchID string) (map[string]string, error) {
	return c.CachedRawPatches, nil
}

func verifyPullRequestEventForAbort(event *github.PullRequestEvent) (string, string, error) {
	if event.Number == nil || event.Repo == nil ||
		event.Repo.FullName == nil || event.PullRequest == nil ||
		event.PullRequest.ClosedAt == nil {
		return "", "", gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "pull request data is malformed",
		}
	}

	baseRepo := strings.Split(*event.Repo.FullName, "/")
	if len(baseRepo) != 2 {
		return "", "", gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "repository name is invalid",
		}
	}

	return baseRepo[0], baseRepo[1], nil
}

func (pc *MockPatchConnector) IsPatchEmpty(id string) (bool, error) {
	return false, nil
}
