package data

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// DBPatchConnector is a struct that implements the Patch related methods
// from the Connector through interactions with the backing database.
type DBPatchConnector struct{}

// FindPatchesByProject uses the service layer's patches type to query the backing database for
// the patches.
func (pc *DBPatchConnector) FindPatchesByProject(projectId string, ts time.Time, limit int, sortDir int) ([]patch.Patch, error) {
	patches, err := patch.Find(patch.PatchesByProject(projectId, ts, limit, sortDir))
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

// MockPatchConnector is a struct that implements the Patch related methods
// from the Connector through interactions with he backing database.
type MockPatchConnector struct {
	CachedPatches []patch.Patch
}

// FindPatchesByProject queries the cached patches splice for the matching patches.
// Assumes CachedPatches is sorted by increasing creation time.
func (hp *MockPatchConnector) FindPatchesByProject(projectId string, ts time.Time, limit int, sort int) ([]patch.Patch, error) {
	patchesToReturn := []patch.Patch{}
	if limit <= 0 {
		return patchesToReturn, nil
	}
	if sort > 0 {
		for i := 0; i < len(hp.CachedPatches); i++ {
			p := hp.CachedPatches[i]
			if p.Project == projectId && !p.CreateTime.Before(ts) {
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
	for _, p := range pc.CachedPatches {
		if p.Id.Hex() == patchId {
			return &p, nil
		}
	}
	return nil, &rest.APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("patch with id %s not found", patchId),
	}
}
