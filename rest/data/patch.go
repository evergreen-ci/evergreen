package data

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/google/go-github/v34/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
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
