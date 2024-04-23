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
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func ValidatePatchID(patchId string) error {
	if !mgobson.IsObjectIdHex(patchId) {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("patch ID '%s' is invalid because it is not an ObjectId", patchId),
		}
	}

	return nil
}

// FindPatchesByProject uses the service layer's patches type to query the backing database for
// the patches.
func FindPatchesByProject(projectId string, ts time.Time, limit int) ([]restModel.APIPatch, error) {
	apiPatches := []restModel.APIPatch{}
	id, err := model.GetIdForProject(projectId)
	if err != nil {
		return nil, errors.Wrapf(err, "fetching project '%s'", projectId)
	}
	patches, err := patch.Find(patch.PatchesByProject(id, ts, limit))
	if err != nil {
		return nil, errors.Wrapf(err, "fetching patches for project '%s'", id)
	}
	for _, p := range patches {
		apiPatch := restModel.APIPatch{}
		err = apiPatch.BuildFromService(p, &restModel.APIPatchArgs{
			IncludeProjectIdentifier:   true,
			IncludeCommitQueuePosition: true,
			IncludeChildPatches:        true,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "converting patch '%s' to API model", p.Id.Hex())
		}
		apiPatches = append(apiPatches, apiPatch)
	}

	return apiPatches, nil
}

// FindPatchById queries the backing database for the patch matching patchId.
func FindPatchById(patchId string) (*restModel.APIPatch, error) {
	if err := ValidatePatchID(patchId); err != nil {
		return nil, errors.WithStack(err)
	}

	p, err := patch.FindOneId(patchId)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch '%s' not found", patchId),
		}
	}

	apiPatch := restModel.APIPatch{}
	err = apiPatch.BuildFromService(*p, &restModel.APIPatchArgs{
		IncludeChildPatches:        true,
		IncludeProjectIdentifier:   true,
		IncludeCommitQueuePosition: true,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "converting patch '%s' to API model", p.Id.Hex())
	}

	return &apiPatch, nil
}

// AbortPatch uses the service level CancelPatch method to abort a single patch
// with matching Id.
func AbortPatch(ctx context.Context, patchId string, user string) error {
	if err := ValidatePatchID(patchId); err != nil {
		return errors.WithStack(err)
	}

	p, err := patch.FindOne(patch.ById(mgobson.ObjectIdHex(patchId)))
	if err != nil {
		return err
	}
	if p == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch '%s' not found", patchId),
		}
	}
	return model.CancelPatch(ctx, p, task.AbortInfo{User: user})
}

// SetPatchActivated attempts to activate the patch and create a new version (if activated is set to true)
func SetPatchActivated(ctx context.Context, patchId string, user string, activated bool, settings *evergreen.Settings) error {
	p, err := patch.FindOne(patch.ById(mgobson.ObjectIdHex(patchId)))
	if err != nil {
		return err
	}
	if p == nil {
		return errors.Errorf("could not find patch '%s'", patchId)
	}
	err = p.SetActivation(activated)
	if err != nil {
		return err
	}
	if activated && p.Version == "" {
		requester := p.GetRequester()

		token, err := settings.GetGithubOauthToken()
		if err != nil {
			return errors.Wrap(err, "getting GitHub OAuth token from admin settings")
		}
		if _, err = model.FinalizePatch(ctx, p, requester, token); err != nil {
			return errors.Wrapf(err, "finalizing patch '%s'", p.Id.Hex())
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
				return errors.Wrap(err, "adding GitHub status update job to queue")
			}
		}
	}

	return model.SetVersionActivation(ctx, patchId, activated, user)
}

// FindPatchesByUser finds patches for the input user as ordered by creation time
func FindPatchesByUser(user string, ts time.Time, limit int) ([]restModel.APIPatch, error) {
	patches, err := patch.Find(patch.ByUserPaginated(user, ts, limit))
	if err != nil {
		return nil, errors.Wrapf(err, "fetching patches for user '%s'", user)
	}
	apiPatches := []restModel.APIPatch{}
	for _, p := range patches {
		apiPatch := restModel.APIPatch{}
		err = apiPatch.BuildFromService(p, &restModel.APIPatchArgs{
			IncludeProjectIdentifier:   true,
			IncludeCommitQueuePosition: true,
			IncludeChildPatches:        true,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "converting patch '%s' to API model", p.Id.Hex())
		}
		apiPatches = append(apiPatches, apiPatch)
	}

	return apiPatches, nil
}

// AbortPatchesFromPullRequest aborts patches with the same PR Number,
// in the same repository, at the pull request's close time. This returns
// whether or not one of the GitHub patches is a commit queue item and is
// currently merging the PR.
func AbortPatchesFromPullRequest(ctx context.Context, event *github.PullRequestEvent) error {
	owner, repo, err := verifyPullRequestEventForAbort(event)
	if err != nil {
		return err
	}

	if err = model.AbortPatchesWithGithubPatchData(ctx, event.PullRequest.GetClosedAt().Time, true, "", owner, repo, *event.Number); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "aborting patches").Error(),
		}
	}

	return nil
}

// GetRawPatches fetches the raw patches for a patch.
func GetRawPatches(patchID string) (*restModel.APIRawPatch, error) {
	patchDoc, err := patch.FindOneId(patchID)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding patch '%s'", patchID).Error(),
		}
	}
	if patchDoc == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch '%s' not found", patchID),
		}
	}

	if err = patchDoc.FetchPatchFiles(false); err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "getting patch contents").Error(),
		}
	}
	var rawPatch restModel.APIRawPatch
	for _, raw := range patchDoc.Patches {
		module := restModel.APIRawModule{
			Name:    raw.ModuleName,
			Diff:    raw.PatchSet.Patch,
			Githash: raw.Githash,
		}
		if raw.ModuleName == "" {
			rawPatch.Patch = module
		} else {
			rawPatch.RawModules = append(rawPatch.RawModules, module)
		}
	}
	if rawPatch.Patch.Githash == "" {
		// If there aren't any changes for the base project, we should still add this to the list,
		// using the patch githash.
		rawPatch.Patch.Githash = patchDoc.Githash
	}

	return &rawPatch, nil
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
