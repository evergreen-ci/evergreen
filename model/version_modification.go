package model

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type VersionModification struct {
	Action            evergreen.ModificationAction `json:"action"`
	Active            bool                         `json:"active"`
	Abort             bool                         `json:"abort"`
	Priority          int64                        `json:"priority"`
	VersionsToRestart []*VersionToRestart          `json:"versions_to_restart"`
	TaskIds           []string                     `json:"task_ids"` // deprecated
}

func ModifyVersion(version Version, user user.DBUser, modifications VersionModification) (int, error) {
	switch modifications.Action {
	case evergreen.RestartAction:
		if modifications.VersionsToRestart == nil { // To maintain backwards compatibility with legacy UI
			if err := RestartVersion(version.Id, modifications.TaskIds, modifications.Abort, user.Id); err != nil {
				return http.StatusInternalServerError, errors.Wrap(err, "restarting patch")
			}
		}
		if err := RestartVersions(modifications.VersionsToRestart, modifications.Abort, user.Id); err != nil {
			return http.StatusInternalServerError, errors.Wrap(err, "restarting patch")
		}
	case evergreen.SetActiveAction:
		if version.Requester == evergreen.MergeTestRequester && modifications.Active {
			return http.StatusBadRequest, errors.New("commit queue merges cannot be manually scheduled")
		}
		if err := SetVersionActivation(version.Id, modifications.Active, user.Id); err != nil {
			return http.StatusInternalServerError, errors.Wrap(err, "activating patch")
		}

		// abort after deactivating the version so we aren't bombarded with failing tasks while
		// the deactivation is in progress
		if modifications.Abort {
			if err := task.AbortVersion(version.Id, task.AbortInfo{User: user.DisplayName()}); err != nil {
				return http.StatusInternalServerError, errors.Wrap(err, "aborting patch")
			}
		}
		if !modifications.Active && version.Requester == evergreen.MergeTestRequester {
			err := RestartItemsAfterVersion(nil, version.Identifier, version.Id, user.Id)
			if err != nil {
				return http.StatusInternalServerError, errors.Wrap(err, "restarting later commit queue items")
			}
			_, err = RemoveCommitQueueItemForVersion(version.Identifier, version.Id, user.DisplayName())
			if err != nil {
				return http.StatusInternalServerError, errors.Wrap(err, "removing patch from commit queue")
			}
			p, err := patch.FindOneId(version.Id)
			if err != nil {
				return http.StatusInternalServerError, errors.Wrap(err, "finding patch")
			}
			if p == nil {
				return http.StatusNotFound, errors.New("patch not found")
			}
			err = SendCommitQueueResult(p, message.GithubStateError, fmt.Sprintf("deactivated by user '%s'", user.DisplayName()))
			grip.Error(message.WrapError(err, message.Fields{
				"message": "unable to send github status",
				"patch":   version.Id,
			}))

		}
	case evergreen.SetPriorityAction:
		projId := version.Identifier
		if projId == "" {
			return http.StatusNotFound, errors.Errorf("could not find project for version '%s'", version.Id)
		}
		if modifications.Priority > evergreen.MaxTaskPriority {
			requiredPermission := gimlet.PermissionOpts{
				Resource:      projId,
				ResourceType:  "project",
				Permission:    evergreen.PermissionTasks,
				RequiredLevel: evergreen.TasksAdmin.Value,
			}
			if !user.HasPermission(requiredPermission) {
				return http.StatusUnauthorized, errors.Errorf("insufficient access to set priority %d, can only set priority less than or equal to %d", modifications.Priority, evergreen.MaxTaskPriority)
			}
		}
		if err := SetVersionsPriority([]string{version.Id}, modifications.Priority, user.Id); err != nil {
			return http.StatusInternalServerError, errors.Wrap(err, "setting version priority")
		}
	default:
		return http.StatusBadRequest, errors.Errorf("unrecognized action '%s'", modifications.Action)
	}
	return 0, nil
}
