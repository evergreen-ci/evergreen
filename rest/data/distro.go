package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// FindDistroById queries the database to find a given distros.
func FindDistroById(distroId string) (*distro.Distro, error) {
	d, err := distro.FindOne(distro.ById(distroId))
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro with id '%s' not found", distroId),
		}
	}
	return &d, nil
}

// FindAllDistros queries the database to find all distros.
func FindAllDistros() ([]distro.Distro, error) {
	distros, err := distro.Find(distro.All)
	if err != nil {
		return nil, err
	}
	if distros == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no distros found"),
		}
	}
	return distros, nil
}

// UpdateDistro updates the given distro.Distro.
func UpdateDistro(old, new *distro.Distro) error {
	if old.Id != new.Id {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("programmer error updating distro %s [%s]", new.Id, old.Id),
		}

	}

	if old.DispatcherSettings.Version == evergreen.DispatcherVersionRevisedWithDependencies && new.DispatcherSettings.Version != evergreen.DispatcherVersionRevisedWithDependencies {
		if err := model.RemoveTaskQueues(new.Id); err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("could not clear invalid task queues for %s", new.Id),
			}
		}
	}
	err := new.Update()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("distro with id '%s' was not updated", new.Id),
		}
	}
	return nil
}

// CreateDistro inserts the given distro.Distro.
func CreateDistro(distro *distro.Distro) error {
	err := distro.Insert()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("distro with id '%s' was not inserted", distro.Id),
		}
	}
	return nil
}

// DeleteDistroById removes a given distro from the database based on its id.
func DeleteDistroById(distroId string) error {
	d, err := distro.FindByID(distroId)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("problem finding distro '%s'", distroId),
		}
	}
	if d == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("distro '%s' doesn't exist", distroId),
		}
	}
	if err = host.MarkInactiveStaticHosts([]string{}, d); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("hosts for distro with id '%s' were not terminated", distroId),
		}
	}
	if err = distro.Remove(distroId); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("distro with id '%s' was not deleted", distroId),
		}
	}
	if err = model.ClearTaskQueue(distroId); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("failed to clear task queue for distro '%s'", distroId),
		}
	}
	return nil
}

func getDefaultProviderSettings(d *distro.Distro) (map[string]interface{}, error) {
	var doc *birch.Document
	var err error
	if len(d.ProviderSettingsList) == 1 {
		doc = d.ProviderSettingsList[0]
	} else if len(d.ProviderSettingsList) > 1 {
		doc, err = d.GetProviderSettingByRegion(evergreen.DefaultEC2Region)
		if err != nil {
			return nil, errors.Wrapf(err, "providers list doesn't contain region '%s'", evergreen.DefaultEC2Region)
		}
	}
	if doc != nil {
		return doc.ExportMap(), nil
	}
	return nil, nil
}

// ClearTaskQueue deletes all tasks from the task queue for a distro
func ClearTaskQueue(distroId string) error {
	return model.ClearTaskQueue(distroId)
}
