package model

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
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

// ModifyVersion handles making particular changes to the version, such as restarting, setting active, etc,
// as well as for child patches if relevant.
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
			if err := AbortVersion(version.Id, task.AbortInfo{User: user.DisplayName()}); err != nil {
				return http.StatusInternalServerError, errors.Wrap(err, "aborting patch")
			}
		}
		if !modifications.Active && version.Requester == evergreen.MergeTestRequester {
			cq, err := commitqueue.FindOneId(version.Identifier)
			if err != nil {
				return http.StatusInternalServerError, errors.Wrapf(err, "finding commit queue '%s'", version.Identifier)
			}
			if cq == nil {
				return http.StatusNotFound, errors.Errorf("commit queue '%s' for version '%s' not found", version.Identifier, version.Id)
			}
			if _, err := DequeueAndRestartForVersion(cq, version.Identifier, version.Id, user.Id, "merge task is being deactivated"); err != nil {
				return http.StatusInternalServerError, err
			}
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
		versionIds := []string{version.Id}
		if evergreen.IsPatchRequester(version.Requester) {
			// Only modify the child patch if it is finalized.
			childPatchIds, err := patch.GetFinalizedChildPatchIdsForPatch(version.Id)
			if err != nil {
				return http.StatusNotFound, errors.Wrap(err, "finding finalized patch ids")
			}
			versionIds = append(versionIds, childPatchIds...)
		}
		if err := SetVersionsPriority(versionIds, modifications.Priority, user.Id); err != nil {
			return http.StatusInternalServerError, errors.Wrap(err, "setting version priority")
		}
	default:
		return http.StatusBadRequest, errors.Errorf("unrecognized action '%s'", modifications.Action)
	}
	return 0, nil
}
