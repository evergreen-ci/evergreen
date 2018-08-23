package model

import (
	"errors"
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
)

type APIProject struct {
	BatchTime          int         `json:"batch_time"`
	Branch             APIString   `json:"branch_name"`
	DisplayName        APIString   `json:"display_name"`
	Enabled            bool        `json:"enabled"`
	Identifier         APIString   `json:"identifier"`
	Owner              APIString   `json:"owner_name"`
	Private            bool        `json:"private"`
	RemotePath         APIString   `json:"remote_path"`
	Repo               APIString   `json:"repo_name"`
	Tracked            bool        `json:"tracked"`
	DeactivatePrevious bool        `json:"deactivate_previous"`
	Admins             []APIString `json:"admins"`
	TracksPushEvents   bool        `json:"tracks_push_events"`
	PRTestingEnabled   bool        `json:"pr_testing_enabled"`
}

func (apiProject *APIProject) BuildFromService(p interface{}) error {
	v, ok := p.(model.ProjectRef)
	if !ok {
		return fmt.Errorf("incorrect type when fetching converting project type")
	}
	apiProject.BatchTime = v.BatchTime
	apiProject.Branch = ToAPIString(v.Branch)
	apiProject.DisplayName = ToAPIString(v.DisplayName)
	apiProject.Enabled = v.Enabled
	apiProject.Identifier = ToAPIString(v.Identifier)
	apiProject.Owner = ToAPIString(v.Owner)
	apiProject.Private = v.Private
	apiProject.RemotePath = ToAPIString(v.RemotePath)
	apiProject.Repo = ToAPIString(v.Repo)
	apiProject.Tracked = v.Tracked
	apiProject.TracksPushEvents = v.TracksPushEvents
	apiProject.PRTestingEnabled = v.PRTestingEnabled
	apiProject.DeactivatePrevious = v.DeactivatePrevious

	admins := []APIString{}
	for _, a := range v.Admins {
		admins = append(admins, ToAPIString(a))
	}
	apiProject.Admins = admins

	return nil
}

func (apiProject *APIProject) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}
