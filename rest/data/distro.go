package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/gimlet"
)

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

// DeleteDistroById removes a given distro from the database based on its id.
func DeleteDistroById(distroId string) error {
	d, err := distro.FindOneId(distroId)
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
