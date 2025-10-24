package model

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
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
}

func ModifyVersion(ctx context.Context, version Version, user user.DBUser, modifications VersionModification) (int, error) {
	switch modifications.Action {
	case evergreen.RestartAction:
		if version.Requester == evergreen.GithubMergeRequester {
			return http.StatusBadRequest, errors.New("merge queue patches cannot be manually restarted")
		}
		if err := RestartVersions(ctx, modifications.VersionsToRestart, modifications.Abort, user.Id); err != nil {
			return http.StatusInternalServerError, errors.Wrap(err, "restarting patch")
		}
	case evergreen.SetActiveAction:
		if version.Requester == evergreen.GithubMergeRequester {
			return http.StatusBadRequest, errors.New("merge queue patches cannot be manually scheduled")
		}
		if err := SetVersionActivation(ctx, version.Id, modifications.Active, user.Id); err != nil {
			return http.StatusInternalServerError, errors.Wrap(err, "activating patch")
		}

		// abort after deactivating the version so we aren't bombarded with failing tasks while
		// the deactivation is in progress
		if modifications.Abort {
			if err := task.AbortVersionTasks(ctx, version.Id, task.AbortInfo{User: user.DisplayName()}); err != nil {
				return http.StatusInternalServerError, errors.Wrap(err, "aborting patch")
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
				return http.StatusUnauthorized, errors.Errorf("not authorized to set priority %d, can only set priority less than or equal to %d", modifications.Priority, evergreen.MaxTaskPriority)
			}
		}
		if err := SetVersionsPriority(ctx, []string{version.Id}, modifications.Priority, user.Id); err != nil {
			return http.StatusInternalServerError, errors.Wrap(err, "setting version priority")
		}
	default:
		return http.StatusBadRequest, errors.Errorf("unrecognized action '%s'", modifications.Action)
	}
	return 0, nil
}
