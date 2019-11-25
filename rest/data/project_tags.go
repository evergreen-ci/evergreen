package data

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
)

type DBProjectTagConnector struct{}

func (DBProjectTagConnector) FindProjectsByTag(tag string) ([]restModel.APIProjectRef, error) {
	dref, err := model.FindTaggedProjectRefs(false, tag)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}
	if dref == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    err.Error(),
		}
	}

	catcher := grip.NewBasicCatcher()
	out := make([]restModel.APIProjectRef, len(dref))
	for idx := range dref {
		m := restModel.APIProjectRef{}
		catcher.Add(m.BuildFromService(dref[idx]))
		out[idx] = m
	}
	if catcher.HasErrors() {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    catcher.Resolve().Error(),
		}
	}

	return out, nil
}
func (DBProjectTagConnector) AddTagsToProject(id string, tags ...string) error {
	ref, err := model.FindOneProjectRef(id)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	if ref == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no project '%s'", id),
		}
	}

	found, err := ref.AddTags(tags...)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	if !found {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot add tags '%s' to project %s", strings.Join(tags, ", "), id),
		}
	}

	return nil
}

func (DBProjectTagConnector) RemoveTagFromProject(id, tag string) error {
	ref, err := model.FindOneProjectRef(id)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	if ref == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no project '%s'", id),
		}
	}

	found, err := ref.RemoveTag(tag)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	if !found {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("project '%s' does not have tag %s", id, tag),
		}
	}

	return nil
}

type MockProjectTagConnector struct{}

func (MockProjectTagConnector) FindProjectsByTag(tag string) ([]restModel.APIProjectRef, error) {
	return nil, nil
}
func (MockProjectTagConnector) AddTagsToProject(id string, tags ...string) error { return nil }
func (MockProjectTagConnector) RemoveTagFromProject(id, tag string) error        { return nil }
