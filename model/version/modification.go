package version

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type ModificationAction string

const (
	Restart     ModificationAction = "restart"
	SetActive   ModificationAction = "set_active"
	SetPriority ModificationAction = "set_priority"
)

type Modification struct {
	Action            ModificationAction        `json:"action"`
	Active            bool                      `json:"active"`
	Abort             bool                      `json:"abort"`
	Priority          int64                     `json:"priority"`
	VersionsToRestart []*model.VersionToRestart `json:"versions_to_restart"`
	TaskIds           []string                  `json:"task_ids"` // deprecated
}

func ModifyVersion(version model.Version, user user.DBUser, modifications Modification) (int, error) {
	switch modifications.Action {
	case Restart:
		if modifications.VersionsToRestart == nil { // to maintain backwards compatibility with legacy Ui and support the deprecated restartPatch resolver
			if err := model.RestartVersion(version.Id, modifications.TaskIds, modifications.Abort, user.Id); err != nil {
				return http.StatusInternalServerError, errors.Errorf("error restarting patch: %s", err)
			}
		}
		if err := model.RestartVersions(modifications.VersionsToRestart, modifications.Abort, user.Id); err != nil {
			return http.StatusInternalServerError, errors.Errorf("error restarting patch: %s", err)
		}
	case SetActive:
		if version.Requester == evergreen.MergeTestRequester && modifications.Active {
			return http.StatusBadRequest, errors.New("commit queue merges cannot be manually scheduled")
		}
		if err := model.SetVersionActivation(version.Id, modifications.Active, user.Id); err != nil {
			return http.StatusInternalServerError, errors.Errorf("error activating patch: %s", err)
		}
		// abort after deactivating the version so we aren't bombarded with failing tasks while
		// the deactivation is in progress
		if modifications.Abort {
			if err := task.AbortVersion(version.Id, task.AbortInfo{User: user.DisplayName()}); err != nil {
				return http.StatusInternalServerError, errors.Errorf("error aborting patch: %s", err)
			}
		}
		if !modifications.Active && version.Requester == evergreen.MergeTestRequester {
			err := model.RestartItemsAfterVersion(nil, version.Identifier, version.Id, user.Id)
			if err != nil {
				return http.StatusInternalServerError, errors.Errorf("error restarting later commit queue items: %s", err)
			}
			_, err = commitqueue.RemoveCommitQueueItemForVersion(version.Identifier, version.Id, user.DisplayName())
			if err != nil {
				return http.StatusInternalServerError, errors.Errorf("error removing patch from commit queue: %s", err)
			}
			p, err := patch.FindOneId(version.Id)
			if err != nil {
				return http.StatusInternalServerError, errors.Wrap(err, "unable to find patch")
			}
			if p == nil {
				return http.StatusNotFound, errors.New("patch not found")
			}
			err = model.SendCommitQueueResult(p, message.GithubStateError, fmt.Sprintf("deactivated by '%s'", user.DisplayName()))
			grip.Error(message.WrapError(err, message.Fields{
				"message": "unable to send github status",
				"patch":   version.Id,
			}))

		}
	case SetPriority:
		projId := version.Identifier
		if projId == "" {
			return http.StatusNotFound, errors.Errorf("Could not find project for version %s", version.Id)
		}
		if modifications.Priority > evergreen.MaxTaskPriority {
			requiredPermission := gimlet.PermissionOpts{
				Resource:      projId,
				ResourceType:  "project",
				Permission:    evergreen.PermissionTasks,
				RequiredLevel: evergreen.TasksAdmin.Value,
			}
			if !user.HasPermission(requiredPermission) {
				return http.StatusUnauthorized, errors.Errorf("Insufficient access to set priority %v, can only set priority less than or equal to %v", modifications.Priority, evergreen.MaxTaskPriority)
			}
		}
		if err := model.SetVersionPriority(version.Id, modifications.Priority, user.Id); err != nil {
			return http.StatusInternalServerError, errors.Errorf("error setting version priority: %s", err)
		}
	default:
		return http.StatusBadRequest, errors.Errorf("Unrecognized action: %v", modifications.Action)
	}
	return 0, nil
}
