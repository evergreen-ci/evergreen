package data

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// DBPatchConnector is a struct that implements the Patch related methods
// from the Connector through interactions with the backing database.
type DBPatchConnector struct{}

// FindPatchesByProject uses the service layer's patches type to query the backing database for
// the patches.
func (pc *DBPatchConnector) FindPatchesByProject(projectId string, ts time.Time, limit int, sortAsc bool) ([]patch.Patch, error) {
	patches, err := patch.Find(patch.PatchesByProject(projectId, ts, limit, sortAsc))
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching patches for project %s", projectId)
	}

	return patches, nil
}

// FindPatchById queries the backing database for the patch matching patchId.
func (pc *DBPatchConnector) FindPatchById(patchId string) (*patch.Patch, error) {
	p, err := patch.FindOne(patch.ById(bson.ObjectIdHex(patchId)))
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch with id %s not found", patchId),
		}
	}
	return p, nil
}

// AbortPatch uses the service level CancelPatch method to abort a single patch
// with matching Id.
func (pc *DBPatchConnector) AbortPatch(patchId string, user string) error {
	p, err := patch.FindOne(patch.ById(bson.ObjectIdHex(patchId)))
	if err != nil {
		return err
	}
	if p == nil {
		return &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch with id %s not found", patchId),
		}
	}
	return model.CancelPatch(p, user)
}

// SetPatchPriority attempts to set the priority on the corresponding version.
// Will not error if no version exists.
func (pc *DBPatchConnector) SetPatchPriority(patchId string, priority int64) error {
	return model.SetVersionPriority(patchId, priority)
}

// SetPatchActivated attempts to set the priority on the patch and the corresponding
// version. Will not error if no version exists.
func (pc *DBPatchConnector) SetPatchActivated(patchId string, user string, activated bool) error {
	p, err := pc.FindPatchById(patchId)
	if err != nil {
		return err
	}
	err = p.SetActivation(activated)
	if err != nil {
		return err
	}
	return model.SetVersionActivation(patchId, activated, user)
}

func (pc *DBPatchConnector) FindPatchesByUser(user string, ts time.Time, limit int, sortAsc bool) ([]patch.Patch, error) {
	patches, err := patch.Find(patch.ByUserPaginated(user, ts, limit, sortAsc))
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching patches for user %s", user)
	}

	return patches, nil
}

func (p *DBPatchConnector) AbortPatchesFromPullRequest(event *github.PullRequestEvent) error {
	owner, repo, err := verifyPullRequestEventForAbort(event)
	if err != nil {
		return err
	}

	err = model.AbortPatchesWithGithubPatchData(*event.PullRequest.ClosedAt,
		owner, repo, *event.Number)
	if err != nil {
		return &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "error aborting patches",
		}
	}

	return nil
}

// MockPatchConnector is a struct that implements the Patch related methods
// from the Connector through interactions with he backing database.
type MockPatchConnector struct {
	CachedPatches  []patch.Patch
	CachedAborted  map[string]string
	CachedPriority map[string]int64
}

// FindPatchesByProject queries the cached patches splice for the matching patches.
// Assumes CachedPatches is sorted by increasing creation time.
func (hp *MockPatchConnector) FindPatchesByProject(projectId string, ts time.Time, limit int, sortAsc bool) ([]patch.Patch, error) {
	patchesToReturn := []patch.Patch{}
	if limit <= 0 {
		return patchesToReturn, nil
	}
	if sortAsc {
		for i := 0; i < len(hp.CachedPatches); i++ {
			p := hp.CachedPatches[i]
			if p.Project == projectId && p.CreateTime.After(ts) {
				patchesToReturn = append(patchesToReturn, p)
				if len(patchesToReturn) == limit {
					break
				}
			}
		}
	} else {
		for i := len(hp.CachedPatches) - 1; i >= 0; i-- {
			p := hp.CachedPatches[i]
			if p.Project == projectId && !p.CreateTime.After(ts) {
				patchesToReturn = append(patchesToReturn, p)
				if len(patchesToReturn) == limit {
					break
				}
			}
		}
	}
	return patchesToReturn, nil
}

// FindPatchById iterates through the slice of CachedPatches to find the matching patch.
func (pc *MockPatchConnector) FindPatchById(patchId string) (*patch.Patch, error) {
	for idx := range pc.CachedPatches {
		if pc.CachedPatches[idx].Id.Hex() == patchId {
			return &pc.CachedPatches[idx], nil
		}
	}
	return nil, &rest.APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("patch with id %s not found", patchId),
	}
}

// AbortPatch sets the value of patchId in CachedAborted to user.
func (pc *MockPatchConnector) AbortPatch(patchId string, user string) error {
	var foundPatch *patch.Patch
	var foundIdx int
	for idx, p := range pc.CachedPatches {
		if p.Id.Hex() == patchId {
			foundPatch = &p
			foundIdx = idx
			break
		}
	}
	if foundPatch == nil {
		return &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch with id %s not found", patchId),
		}
	}
	pc.CachedAborted[patchId] = user
	if foundPatch.Version == "" {
		pc.CachedPatches = append(pc.CachedPatches[:foundIdx], pc.CachedPatches[foundIdx+1:]...)
	}
	return nil
}

// SetPatchPriority sets the patch priority in the CachedPriority map.
func (pc *MockPatchConnector) SetPatchPriority(patchId string, priority int64) error {
	pc.CachedPriority[patchId] = priority
	return nil
}

// SetPatchActivated sets the boolean activated field on the input patch.
func (pc *MockPatchConnector) SetPatchActivated(patchId string, user string, activated bool) error {
	p, err := pc.FindPatchById(patchId)
	if err != nil {
		return err
	}
	p.Activated = activated
	return nil
}

// FindPatchesByUser iterates through the cached patches slice to find the correct patches
func (hp *MockPatchConnector) FindPatchesByUser(user string, ts time.Time, limit int, sortAsc bool) ([]patch.Patch, error) {
	patchesToReturn := []patch.Patch{}
	if limit <= 0 {
		return patchesToReturn, nil
	}
	if sortAsc {
		for i := 0; i < len(hp.CachedPatches); i++ {
			p := hp.CachedPatches[i]
			if p.Author == user && p.CreateTime.After(ts) {
				patchesToReturn = append(patchesToReturn, p)
				if len(patchesToReturn) == limit {
					break
				}
			}
		}
	} else {
		for i := len(hp.CachedPatches) - 1; i >= 0; i-- {
			p := hp.CachedPatches[i]
			if p.Author == user && !p.CreateTime.After(ts) {
				patchesToReturn = append(patchesToReturn, p)
				if len(patchesToReturn) == limit {
					break
				}
			}
		}
	}
	return patchesToReturn, nil
}

func (c *MockPatchConnector) AbortPatchesFromPullRequest(event *github.PullRequestEvent) error {
	_, _, err := verifyPullRequestEventForAbort(event)
	return err
}

func verifyPullRequestEventForAbort(event *github.PullRequestEvent) (string, string, error) {
	if event.Number == nil || event.Repo == nil ||
		event.Repo.FullName == nil || event.PullRequest == nil ||
		event.PullRequest.ClosedAt == nil {
		return "", "", &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "pull request data is malformed",
		}
	}

	baseRepo := strings.Split(*event.Repo.FullName, "/")
	if len(baseRepo) != 2 {
		return "", "", &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "repository name is invalid",
		}
	}

	return baseRepo[0], baseRepo[1], nil
}
