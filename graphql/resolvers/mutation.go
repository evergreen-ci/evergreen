package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"sort"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/api"
	"github.com/evergreen-ci/evergreen/cloud"
	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	"github.com/evergreen-ci/evergreen/graphql/resolvers/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	evgUtil "github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	werrors "github.com/pkg/errors"
)

func (r *mutationResolver) BbCreateTicket(ctx context.Context, taskID string, execution *int) (bool, error) {
	httpStatus, err := data.BbFileTicket(ctx, taskID, *execution)
	if err != nil {
		return false, util.MapHTTPStatusToGqlError(ctx, httpStatus, err)
	}
	return true, nil
}

func (r *mutationResolver) AddAnnotationIssue(ctx context.Context, taskID string, execution int, apiIssue restModel.APIIssueLink, isIssue bool) (bool, error) {
	usr := util.MustHaveUser(ctx)
	issue := restModel.APIIssueLinkToService(apiIssue)
	if err := evgUtil.CheckURL(issue.URL); err != nil {
		return false, gqlError.InputValidationError.Send(ctx, fmt.Sprintf("issue does not have valid URL: %s", err.Error()))
	}
	if isIssue {
		if err := annotations.AddIssueToAnnotation(taskID, execution, *issue, usr.Username()); err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't add issue: %s", err.Error()))
		}
		return true, nil
	} else {
		if err := annotations.AddSuspectedIssueToAnnotation(taskID, execution, *issue, usr.Username()); err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't add suspected issue: %s", err.Error()))
		}
		return true, nil
	}
}

func (r *mutationResolver) EditAnnotationNote(ctx context.Context, taskID string, execution int, originalMessage string, newMessage string) (bool, error) {
	usr := util.MustHaveUser(ctx)
	if err := annotations.UpdateAnnotationNote(taskID, execution, originalMessage, newMessage, usr.Username()); err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't update note: %s", err.Error()))
	}
	return true, nil
}

func (r *mutationResolver) MoveAnnotationIssue(ctx context.Context, taskID string, execution int, apiIssue restModel.APIIssueLink, isIssue bool) (bool, error) {
	usr := util.MustHaveUser(ctx)
	issue := restModel.APIIssueLinkToService(apiIssue)
	if isIssue {
		if err := annotations.MoveIssueToSuspectedIssue(taskID, execution, *issue, usr.Username()); err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't move issue to suspected issues: %s", err.Error()))
		}
		return true, nil
	} else {
		if err := annotations.MoveSuspectedIssueToIssue(taskID, execution, *issue, usr.Username()); err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't move issue to suspected issues: %s", err.Error()))
		}
		return true, nil
	}
}

func (r *mutationResolver) RemoveAnnotationIssue(ctx context.Context, taskID string, execution int, apiIssue restModel.APIIssueLink, isIssue bool) (bool, error) {
	issue := restModel.APIIssueLinkToService(apiIssue)
	if isIssue {
		if err := annotations.RemoveIssueFromAnnotation(taskID, execution, *issue); err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't delete issue: %s", err.Error()))
		}
		return true, nil
	} else {
		if err := annotations.RemoveSuspectedIssueFromAnnotation(taskID, execution, *issue); err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't delete suspected issue: %s", err.Error()))
		}
		return true, nil
	}
}

func (r *mutationResolver) ReprovisionToNew(ctx context.Context, hostIds []string) (int, error) {
	user := util.MustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(user, hostIds)
	if err != nil {
		return 0, util.MapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetReprovisionToNewCallback(ctx, evergreen.GetEnvironment(), user.Username()))
	if err != nil {
		return 0, util.MapHTTPStatusToGqlError(ctx, httpStatus, werrors.Errorf("Error marking selected hosts as needing to reprovision: %s", err.Error()))
	}

	return hostsUpdated, nil
}

func (r *mutationResolver) RestartJasper(ctx context.Context, hostIds []string) (int, error) {
	user := util.MustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(user, hostIds)
	if err != nil {
		return 0, util.MapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetRestartJasperCallback(ctx, evergreen.GetEnvironment(), user.Username()))
	if err != nil {
		return 0, util.MapHTTPStatusToGqlError(ctx, httpStatus, werrors.Errorf("Error marking selected hosts as needing Jasper service restarted: %s", err.Error()))
	}

	return hostsUpdated, nil
}

func (r *mutationResolver) UpdateHostStatus(ctx context.Context, hostIds []string, status string, notes *string) (int, error) {
	user := util.MustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(user, hostIds)
	if err != nil {
		return 0, util.MapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	rq := evergreen.GetEnvironment().RemoteQueue()
	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetUpdateHostStatusCallback(ctx, evergreen.GetEnvironment(), rq, status, *notes, user))
	if err != nil {
		return 0, util.MapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	return hostsUpdated, nil
}

func (r *mutationResolver) EnqueuePatch(ctx context.Context, patchID string, commitMessage *string) (*restModel.APIPatch, error) {
	user := util.MustHaveUser(ctx)
	existingPatch, err := data.FindPatchById(patchID)
	if err != nil {
		gimletErr, ok := err.(gimlet.ErrorResponse)
		if ok {
			return nil, util.MapHTTPStatusToGqlError(ctx, gimletErr.StatusCode, err)
		}
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error getting patch '%s'", patchID))
	}

	if !util.HasEnqueuePatchPermission(user, existingPatch) {
		return nil, gqlError.Forbidden.Send(ctx, "can't enqueue another user's patch")
	}

	if commitMessage == nil {
		commitMessage = existingPatch.Description
	}

	newPatch, err := data.CreatePatchForMerge(ctx, patchID, utility.FromStringPtr(commitMessage))
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error creating new patch: %s", err.Error()))
	}
	item := restModel.APICommitQueueItem{
		Issue:   newPatch.Id,
		PatchId: newPatch.Id,
		Source:  utility.ToStringPtr(commitqueue.SourceDiff)}
	_, err = data.EnqueueItem(utility.FromStringPtr(newPatch.ProjectId), item, false)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error enqueuing new patch: %s", err.Error()))
	}
	return newPatch, nil
}

func (r *mutationResolver) SchedulePatch(ctx context.Context, patchID string, configure gqlModel.PatchConfigure) (*restModel.APIPatch, error) {
	patchUpdateReq := util.BuildFromGqlInput(configure)
	version, err := model.VersionFindOneId(patchID)
	if err != nil && !adb.ResultsNotFound(err) {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error occurred fetching patch `%s`: %s", patchID, err.Error()))
	}
	statusCode, err := units.SchedulePatch(ctx, patchID, version, patchUpdateReq)
	if err != nil {
		return nil, util.MapHTTPStatusToGqlError(ctx, statusCode, werrors.Errorf("Error scheduling patch `%s`: %s", patchID, err.Error()))
	}
	scheduledPatch, err := data.FindPatchById(patchID)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting scheduled patch `%s`: %s", patchID, err))
	}
	return scheduledPatch, nil
}

func (r *mutationResolver) SchedulePatchTasks(ctx context.Context, patchID string) (*string, error) {
	modifications := model.VersionModification{
		Action: evergreen.SetActiveAction,
		Active: true,
		Abort:  false,
	}
	err := util.ModifyVersionHandler(ctx, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *mutationResolver) ScheduleUndispatchedBaseTasks(ctx context.Context, patchID string) ([]*restModel.APITask, error) {
	opts := task.GetTasksByVersionOptions{
		Statuses:                       evergreen.TaskFailureStatuses,
		IncludeExecutionTasks:          true,
		IncludeBaseTasks:               false,
		IncludeBuildVariantDisplayName: false,
	}
	tasks, _, err := task.GetTasksByVersion(patchID, opts)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Could not fetch tasks for patch: %s ", err.Error()))
	}

	scheduledTasks := []*restModel.APITask{}
	tasksToSchedule := make(map[string]bool)

	for _, t := range tasks {
		// If a task is a generated task don't schedule it until we get all of the generated tasks we want to generate
		if t.GeneratedBy == "" {
			// We can ignore an error while fetching tasks because this could just mean the task didn't exist on the base commit.
			baseTask, _ := t.FindTaskOnBaseCommit()
			if baseTask != nil && baseTask.Status == evergreen.TaskUndispatched {
				tasksToSchedule[baseTask.Id] = true
			}
			// If a task is generated lets find its base task if it exists otherwise we need to generate it
		} else if t.GeneratedBy != "" {
			baseTask, _ := t.FindTaskOnBaseCommit()
			// If the task is undispatched or doesn't exist on the base commit then we want to schedule
			if baseTask == nil {
				generatorTask, err := task.FindByIdExecution(t.GeneratedBy, nil)
				if err != nil {
					return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Experienced an error trying to find the generator task: %s", err.Error()))
				}
				if generatorTask != nil {
					baseGeneratorTask, _ := generatorTask.FindTaskOnBaseCommit()
					// If baseGeneratorTask is nil then it didn't exist on the base task and we can't do anything
					if baseGeneratorTask != nil && baseGeneratorTask.Status == evergreen.TaskUndispatched {
						err = baseGeneratorTask.SetGeneratedTasksToActivate(t.BuildVariant, t.DisplayName)
						if err != nil {
							return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Could not activate generated task: %s", err.Error()))
						}
						tasksToSchedule[baseGeneratorTask.Id] = true

					}
				}
			} else if baseTask.Status == evergreen.TaskUndispatched {
				tasksToSchedule[baseTask.Id] = true
			}

		}
	}

	taskIDs := []string{}
	for taskId := range tasksToSchedule {
		taskIDs = append(taskIDs, taskId)
	}
	scheduled, err := util.SetManyTasksScheduled(ctx, r.sc.GetURL(), true, taskIDs...)
	if err != nil {
		return nil, err
	}
	scheduledTasks = append(scheduledTasks, scheduled...)
	// sort scheduledTasks by display name to guarantee the order of the tasks
	sort.Slice(scheduledTasks, func(i, j int) bool {
		return utility.FromStringPtr(scheduledTasks[i].DisplayName) < utility.FromStringPtr(scheduledTasks[j].DisplayName)
	})

	return scheduledTasks, nil
}

func (r *mutationResolver) SetPatchPriority(ctx context.Context, patchID string, priority int) (*string, error) {
	modifications := model.VersionModification{
		Action:   evergreen.SetPriorityAction,
		Priority: int64(priority),
	}
	err := util.ModifyVersionHandler(ctx, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *mutationResolver) UnschedulePatchTasks(ctx context.Context, patchID string, abort bool) (*string, error) {
	modifications := model.VersionModification{
		Action: evergreen.SetActiveAction,
		Active: false,
		Abort:  abort,
	}
	err := util.ModifyVersionHandler(ctx, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *mutationResolver) AddFavoriteProject(ctx context.Context, identifier string) (*restModel.APIProjectRef, error) {
	p, err := model.FindBranchProjectRef(identifier)
	if err != nil || p == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("could not find project '%s'", identifier))
	}

	usr := util.MustHaveUser(ctx)
	err = usr.AddFavoritedProject(identifier)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}
	apiProjectRef := restModel.APIProjectRef{}
	err = apiProjectRef.BuildFromService(p)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error building APIProjectRef from service: %s", err.Error()))
	}
	return &apiProjectRef, nil
}

func (r *mutationResolver) AttachProjectToNewRepo(ctx context.Context, project gqlModel.MoveProjectInput) (*restModel.APIProjectRef, error) {
	usr := util.MustHaveUser(ctx)
	pRef, err := data.FindProjectById(project.ProjectID, false, false)
	if err != nil || pRef == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project: %s : %s", project.ProjectID, err.Error()))
	}
	pRef.Owner = project.NewOwner
	pRef.Repo = project.NewRepo

	if err = pRef.AttachToNewRepo(usr); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error updating owner/repo: %s", err.Error()))
	}

	res := &restModel.APIProjectRef{}
	if err = res.BuildFromService(pRef); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building APIProjectRef: %s", err.Error()))
	}
	return res, nil
}

func (r *mutationResolver) AttachProjectToRepo(ctx context.Context, projectID string) (*restModel.APIProjectRef, error) {
	usr := util.MustHaveUser(ctx)
	pRef, err := data.FindProjectById(projectID, false, false)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding project %s: %s", projectID, err.Error()))
	}
	if pRef == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find project %s", projectID))
	}
	if err = pRef.AttachToRepo(usr); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error attaching to repo: %s", err.Error()))
	}

	res := &restModel.APIProjectRef{}
	if err := res.BuildFromService(pRef); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error building project from service: %s", err.Error()))
	}
	return res, nil
}

func (r *mutationResolver) CreateProject(ctx context.Context, project restModel.APIProjectRef) (*restModel.APIProjectRef, error) {
	i, err := project.ToService()
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("API error converting from model.APIProjectRef to model.ProjectRef: %s ", err.Error()))
	}
	dbProjectRef, ok := i.(*model.ProjectRef)
	if !ok {
		return nil, gqlError.InternalServerError.Send(ctx, werrors.Wrapf(err, "Unexpected type %T for model.ProjectRef", i).Error())
	}

	u := gimlet.GetUser(ctx).(*user.DBUser)
	if err = data.CreateProject(dbProjectRef, u); err != nil {
		apiErr, ok := err.(gimlet.ErrorResponse)
		if ok {
			if apiErr.StatusCode == http.StatusBadRequest {
				return nil, gqlError.InputValidationError.Send(ctx, apiErr.Message)
			}
			// StatusNotFound and other error codes are really internal errors bc we determine this input
			return nil, gqlError.InternalServerError.Send(ctx, apiErr.Message)
		}
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}

	projectRef, err := model.FindBranchProjectRef(*project.Identifier)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error looking in project collection: %s", err.Error()))
	}
	if projectRef == nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding project: %s", err.Error()))
	}
	apiProjectRef := restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(projectRef); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error building APIProjectRef from service: %s", err.Error()))
	}

	return &apiProjectRef, nil
}

func (r *mutationResolver) CopyProject(ctx context.Context, project data.CopyProjectOpts) (*restModel.APIProjectRef, error) {
	projectRef, err := data.CopyProject(ctx, project)
	if projectRef == nil && err != nil {
		apiErr, ok := err.(gimlet.ErrorResponse) // make sure bad request errors are handled correctly; all else should be treated as internal server error
		if ok {
			if apiErr.StatusCode == http.StatusBadRequest {
				return nil, gqlError.InputValidationError.Send(ctx, apiErr.Message)
			}
			// StatusNotFound and other error codes are really internal errors bc we determine this input
			return nil, gqlError.InternalServerError.Send(ctx, apiErr.Message)
		}
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())

	}
	if err != nil {
		// Use AddError to bypass gqlgen restriction that data and errors cannot be returned in the same response
		// https://github.com/99designs/gqlgen/issues/1191
		graphql.AddError(ctx, gqlError.PartialError.Send(ctx, err.Error()))
	}
	return projectRef, nil
}

func (r *mutationResolver) DefaultSectionToRepo(ctx context.Context, projectID string, section gqlModel.ProjectSettingsSection) (*string, error) {
	usr := util.MustHaveUser(ctx)
	if err := model.DefaultSectionToRepo(projectID, model.ProjectPageSection(section), usr.Username()); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error defaulting to repo for section: %s", err.Error()))
	}
	return &projectID, nil
}

func (r *mutationResolver) DetachProjectFromRepo(ctx context.Context, projectID string) (*restModel.APIProjectRef, error) {
	usr := util.MustHaveUser(ctx)
	pRef, err := data.FindProjectById(projectID, false, false)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding project %s: %s", projectID, err.Error()))
	}
	if pRef == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find project %s", projectID))
	}
	if err = pRef.DetachFromRepo(usr); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error detaching from repo: %s", err.Error()))
	}

	res := &restModel.APIProjectRef{}
	if err := res.BuildFromService(pRef); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error building project from service: %s", err.Error()))
	}
	return res, nil
}

func (r *mutationResolver) ForceRepotrackerRun(ctx context.Context, projectID string) (bool, error) {
	ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
	j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), projectID)
	if err := evergreen.GetEnvironment().RemoteQueue().Put(ctx, j); err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error creating Repotracker job: %s", err.Error()))
	}
	return true, nil
}

func (r *mutationResolver) RemoveFavoriteProject(ctx context.Context, identifier string) (*restModel.APIProjectRef, error) {
	p, err := model.FindBranchProjectRef(identifier)
	if err != nil || p == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project: %s", identifier))
	}

	usr := util.MustHaveUser(ctx)
	err = usr.RemoveFavoriteProject(identifier)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error removing project : %s : %s", identifier, err))
	}
	apiProjectRef := restModel.APIProjectRef{}
	err = apiProjectRef.BuildFromService(p)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error building APIProjectRef from service: %s", err.Error()))
	}
	return &apiProjectRef, nil
}

func (r *mutationResolver) SaveProjectSettingsForSection(ctx context.Context, projectSettings *restModel.APIProjectSettings, section gqlModel.ProjectSettingsSection) (*restModel.APIProjectSettings, error) {
	projectId := utility.FromStringPtr(projectSettings.ProjectRef.Id)
	usr := util.MustHaveUser(ctx)
	changes, err := data.SaveProjectSettingsForSection(ctx, projectId, projectSettings, model.ProjectPageSection(section), false, usr.Username())
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}
	return changes, nil
}

func (r *mutationResolver) SaveRepoSettingsForSection(ctx context.Context, repoSettings *restModel.APIProjectSettings, section gqlModel.ProjectSettingsSection) (*restModel.APIProjectSettings, error) {
	projectId := utility.FromStringPtr(repoSettings.ProjectRef.Id)
	usr := util.MustHaveUser(ctx)
	changes, err := data.SaveProjectSettingsForSection(ctx, projectId, repoSettings, model.ProjectPageSection(section), true, usr.Username())
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}
	return changes, nil
}

func (r *mutationResolver) DeactivateStepbackTasks(ctx context.Context, projectID string) (bool, error) {
	usr := util.MustHaveUser(ctx)
	if err := task.DeactivateStepbackTasksForProject(projectID, usr.Username()); err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("deactivating current stepback tasks: %s", err.Error()))
	}
	return true, nil
}

func (r *mutationResolver) AttachVolumeToHost(ctx context.Context, volumeAndHost gqlModel.VolumeHost) (bool, error) {
	statusCode, err := cloud.AttachVolume(ctx, volumeAndHost.VolumeID, volumeAndHost.HostID)
	if err != nil {
		return false, util.MapHTTPStatusToGqlError(ctx, statusCode, err)
	}
	return statusCode == http.StatusOK, nil
}

func (r *mutationResolver) DetachVolumeFromHost(ctx context.Context, volumeID string) (bool, error) {
	statusCode, err := cloud.DetachVolume(ctx, volumeID)
	if err != nil {
		return false, util.MapHTTPStatusToGqlError(ctx, statusCode, err)
	}
	return statusCode == http.StatusOK, nil
}

func (r *mutationResolver) EditSpawnHost(ctx context.Context, spawnHost *gqlModel.EditSpawnHostInput) (*restModel.APIHost, error) {
	var v *host.Volume
	usr := util.MustHaveUser(ctx)
	h, err := host.FindOneByIdOrTag(spawnHost.HostID)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding host by id: %s", err))
	}

	if !host.CanUpdateSpawnHost(h, usr) {
		return nil, gqlError.Forbidden.Send(ctx, "You are not authorized to modify this host")
	}

	opts := host.HostModifyOptions{}
	if spawnHost.DisplayName != nil {
		opts.NewName = *spawnHost.DisplayName
	}
	if spawnHost.NoExpiration != nil {
		opts.NoExpiration = spawnHost.NoExpiration
	}
	if spawnHost.Expiration != nil {
		err = h.SetExpirationTime(*spawnHost.Expiration)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while modifying spawnhost expiration time: %s", err))
		}
	}
	if spawnHost.InstanceType != nil {
		var config *evergreen.Settings
		config, err = evergreen.GetConfig()
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, "unable to retrieve server config")
		}
		allowedTypes := config.Providers.AWS.AllowedInstanceTypes

		err = cloud.CheckInstanceTypeValid(ctx, h.Distro, *spawnHost.InstanceType, allowedTypes)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error validating instance type: %s", err))
		}
		opts.InstanceType = *spawnHost.InstanceType
	}
	if spawnHost.AddedInstanceTags != nil || spawnHost.DeletedInstanceTags != nil {
		addedTags := []host.Tag{}
		deletedTags := []string{}
		for _, tag := range spawnHost.AddedInstanceTags {
			tag.CanBeModified = true
			addedTags = append(addedTags, *tag)
		}
		for _, tag := range spawnHost.DeletedInstanceTags {
			deletedTags = append(deletedTags, tag.Key)
		}
		opts.AddInstanceTags = addedTags
		opts.DeleteInstanceTags = deletedTags
	}
	if spawnHost.Volume != nil {
		v, err = host.FindVolumeByID(*spawnHost.Volume)
		if err != nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Error finding requested volume id: %s", err))
		}
		if v.AvailabilityZone != h.Zone {
			return nil, gqlError.InputValidationError.Send(ctx, "Error mounting volume to spawn host, They must be in the same availability zone.")
		}
		opts.AttachVolume = *spawnHost.Volume
	}
	if spawnHost.PublicKey != nil {
		if utility.FromBoolPtr(spawnHost.SavePublicKey) {
			if err = util.SavePublicKey(ctx, *spawnHost.PublicKey); err != nil {
				return nil, err
			}
		}
		opts.AddKey = spawnHost.PublicKey.Key
		if opts.AddKey == "" {
			opts.AddKey, err = usr.GetPublicKey(spawnHost.PublicKey.Name)
			if err != nil {
				return nil, gqlError.InputValidationError.Send(ctx, fmt.Sprintf("No matching key found for name '%s'", spawnHost.PublicKey.Name))
			}
		}
	}
	if err = cloud.ModifySpawnHost(ctx, evergreen.GetEnvironment(), h, opts); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error modifying spawn host: %s", err))
	}
	if spawnHost.ServicePassword != nil {
		_, err = cloud.SetHostRDPPassword(ctx, evergreen.GetEnvironment(), h, *spawnHost.ServicePassword)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error setting spawn host password: %s", err))
		}
	}

	apiHost := restModel.APIHost{}
	err = apiHost.BuildFromService(h)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
	}
	return &apiHost, nil
}

func (r *mutationResolver) SpawnHost(ctx context.Context, spawnHostInput *gqlModel.SpawnHostInput) (*restModel.APIHost, error) {
	usr := util.MustHaveUser(ctx)
	if spawnHostInput.SavePublicKey {
		if err := util.SavePublicKey(ctx, *spawnHostInput.PublicKey); err != nil {
			return nil, err
		}
	}
	dist, err := distro.FindOneId(spawnHostInput.DistroID)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while trying to find distro with id: %s, err:  `%s`", spawnHostInput.DistroID, err))
	}
	if dist == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find Distro with id: %s", spawnHostInput.DistroID))
	}

	options := &restModel.HostRequestOptions{
		DistroID:             spawnHostInput.DistroID,
		Region:               spawnHostInput.Region,
		KeyName:              spawnHostInput.PublicKey.Key,
		IsVirtualWorkstation: spawnHostInput.IsVirtualWorkStation,
		NoExpiration:         spawnHostInput.NoExpiration,
	}
	if spawnHostInput.SetUpScript != nil {
		options.SetupScript = *spawnHostInput.SetUpScript
	}
	if spawnHostInput.UserDataScript != nil {
		options.UserData = *spawnHostInput.UserDataScript
	}
	if spawnHostInput.HomeVolumeSize != nil {
		options.HomeVolumeSize = *spawnHostInput.HomeVolumeSize
	}
	if spawnHostInput.VolumeID != nil {
		options.HomeVolumeID = *spawnHostInput.VolumeID
	}
	if spawnHostInput.Expiration != nil {
		options.Expiration = spawnHostInput.Expiration
	}

	// passing an empty string taskId is okay as long as a
	// taskId is not required by other spawnHostInput parameters
	var t *task.Task
	if spawnHostInput.TaskID != nil && *spawnHostInput.TaskID != "" {
		options.TaskID = *spawnHostInput.TaskID
		if t, err = task.FindOneId(*spawnHostInput.TaskID); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error occurred finding task %s: %s", *spawnHostInput.TaskID, err.Error()))
		}
	}

	if utility.FromBoolPtr(spawnHostInput.UseProjectSetupScript) {
		if t == nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, "A valid task id must be supplied when useProjectSetupScript is set to true")
		}
		options.UseProjectSetupScript = *spawnHostInput.UseProjectSetupScript
	}
	if utility.FromBoolPtr(spawnHostInput.TaskSync) {
		if t == nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, "A valid task id must be supplied when taskSync is set to true")
		}
		options.TaskSync = *spawnHostInput.TaskSync
	}

	if utility.FromBoolPtr(spawnHostInput.SpawnHostsStartedByTask) {
		if t == nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, "A valid task id must be supplied when SpawnHostsStartedByTask is set to true")
		}
		if err = data.CreateHostsFromTask(t, *usr, spawnHostInput.PublicKey.Key); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error spawning hosts from task: %s : %s", *spawnHostInput.TaskID, err))
		}
	}

	spawnHost, err := data.NewIntentHost(ctx, options, usr, evergreen.GetEnvironment().Settings())
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error spawning host: %s", err))
	}
	if spawnHost == nil {
		return nil, gqlError.InternalServerError.Send(ctx, "An error occurred Spawn host is nil")
	}
	apiHost := restModel.APIHost{}
	if err := apiHost.BuildFromService(spawnHost); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
	}
	return &apiHost, nil
}

func (r *mutationResolver) SpawnVolume(ctx context.Context, spawnVolumeInput gqlModel.SpawnVolumeInput) (bool, error) {
	err := util.ValidateVolumeExpirationInput(ctx, spawnVolumeInput.Expiration, spawnVolumeInput.NoExpiration)
	if err != nil {
		return false, err
	}
	volumeRequest := host.Volume{
		AvailabilityZone: spawnVolumeInput.AvailabilityZone,
		Size:             spawnVolumeInput.Size,
		Type:             spawnVolumeInput.Type,
		CreatedBy:        util.MustHaveUser(ctx).Id,
	}
	vol, statusCode, err := cloud.RequestNewVolume(ctx, volumeRequest)
	if err != nil {
		return false, util.MapHTTPStatusToGqlError(ctx, statusCode, err)
	}
	if vol == nil {
		return false, gqlError.InternalServerError.Send(ctx, "Unable to create volume")
	}
	errorTemplate := "Volume %s has been created but an error occurred."
	var additionalOptions restModel.VolumeModifyOptions
	if spawnVolumeInput.Expiration != nil {
		var newExpiration time.Time
		newExpiration, err = restModel.FromTimePtr(spawnVolumeInput.Expiration)
		if err != nil {
			return false, gqlError.InternalServerError.Send(ctx, werrors.Wrapf(err, errorTemplate, vol.ID).Error())
		}
		additionalOptions.Expiration = newExpiration
	} else if spawnVolumeInput.NoExpiration != nil && *spawnVolumeInput.NoExpiration {
		// this value should only ever be true or nil
		additionalOptions.NoExpiration = true
	}
	err = util.ApplyVolumeOptions(ctx, *vol, additionalOptions)
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to apply expiration options to volume %s: %s", vol.ID, err.Error()))
	}
	if spawnVolumeInput.Host != nil {
		statusCode, err := cloud.AttachVolume(ctx, vol.ID, *spawnVolumeInput.Host)
		if err != nil {
			return false, util.MapHTTPStatusToGqlError(ctx, statusCode, werrors.Wrapf(err, errorTemplate, vol.ID))
		}
	}
	return true, nil
}

func (r *mutationResolver) RemoveVolume(ctx context.Context, volumeID string) (bool, error) {
	statusCode, err := cloud.DeleteVolume(ctx, volumeID)
	if err != nil {
		return false, util.MapHTTPStatusToGqlError(ctx, statusCode, err)
	}
	return statusCode == http.StatusOK, nil
}

func (r *mutationResolver) UpdateSpawnHostStatus(ctx context.Context, hostID string, action gqlModel.SpawnHostStatusActions) (*restModel.APIHost, error) {
	h, err := host.FindOneByIdOrTag(hostID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Error finding host by id: %s", err))
	}
	usr := util.MustHaveUser(ctx)
	env := evergreen.GetEnvironment()

	if !host.CanUpdateSpawnHost(h, usr) {
		return nil, gqlError.Forbidden.Send(ctx, "You are not authorized to modify this host")
	}

	var httpStatus int
	switch action {
	case gqlModel.SpawnHostStatusActionsStart:
		httpStatus, err = data.StartSpawnHost(ctx, env, usr, h)
	case gqlModel.SpawnHostStatusActionsStop:
		httpStatus, err = data.StopSpawnHost(ctx, env, usr, h)
	case gqlModel.SpawnHostStatusActionsTerminate:
		httpStatus, err = data.TerminateSpawnHost(ctx, env, usr, h)
	default:
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find matching status for action : %s", action))
	}
	if err != nil {
		if httpStatus == http.StatusInternalServerError {
			var parsedUrl, _ = url.Parse("/graphql/query")
			grip.Error(message.WrapError(err, message.Fields{
				"method":  "POST",
				"url":     parsedUrl,
				"code":    httpStatus,
				"action":  action,
				"request": gimlet.GetRequestID(ctx),
				"stack":   string(debug.Stack()),
			}))
		}
		return nil, util.MapHTTPStatusToGqlError(ctx, httpStatus, err)
	}
	apiHost := restModel.APIHost{}
	err = apiHost.BuildFromService(h)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
	}
	return &apiHost, nil
}

func (r *mutationResolver) UpdateVolume(ctx context.Context, updateVolumeInput gqlModel.UpdateVolumeInput) (bool, error) {
	volume, err := host.FindVolumeByID(updateVolumeInput.VolumeID)
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding volume by id %s: %s", updateVolumeInput.VolumeID, err.Error()))
	}
	if volume == nil {
		return false, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find volume %s", volume.ID))
	}
	err = util.ValidateVolumeExpirationInput(ctx, updateVolumeInput.Expiration, updateVolumeInput.NoExpiration)
	if err != nil {
		return false, err
	}
	err = util.ValidateVolumeName(ctx, updateVolumeInput.Name)
	if err != nil {
		return false, err
	}
	var updateOptions restModel.VolumeModifyOptions
	if updateVolumeInput.NoExpiration != nil {
		if *updateVolumeInput.NoExpiration {
			// this value should only ever be true or nil
			updateOptions.NoExpiration = true
		} else {
			// this value should only ever be true or nil
			updateOptions.HasExpiration = true
		}
	}
	if updateVolumeInput.Expiration != nil {
		var newExpiration time.Time
		newExpiration, err = restModel.FromTimePtr(updateVolumeInput.Expiration)
		if err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error parsing time %s", err))
		}
		updateOptions.Expiration = newExpiration
	}
	if updateVolumeInput.Name != nil {
		updateOptions.NewName = *updateVolumeInput.Name
	}
	err = util.ApplyVolumeOptions(ctx, *volume, updateOptions)
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to update volume %s: %s", volume.ID, err.Error()))
	}

	return true, nil
}

func (r *mutationResolver) AbortTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	t, err := task.FindOneId(taskID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	user := gimlet.GetUser(ctx).DisplayName()
	err = model.AbortTask(taskID, user)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error aborting task %s: %s", taskID, err.Error()))
	}
	t, err = task.FindOneId(taskID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := util.GetAPITaskFromTask(ctx, r.sc.GetURL(), *t)
	return apiTask, err
}

func (r *mutationResolver) OverrideTaskDependencies(ctx context.Context, taskID string) (*restModel.APITask, error) {
	currentUser := util.MustHaveUser(ctx)
	t, err := task.FindByIdExecution(taskID, nil)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	if err = t.SetOverrideDependencies(currentUser.Username()); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error overriding dependencies for task %s: %s", taskID, err.Error()))
	}
	t.DisplayStatus = t.GetDisplayStatus()
	return util.GetAPITaskFromTask(ctx, r.sc.GetURL(), *t)
}

func (r *mutationResolver) RestartTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	usr := util.MustHaveUser(ctx)
	username := usr.Username()
	if err := model.TryResetTask(taskID, username, evergreen.UIPackage, nil); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error restarting task %s: %s", taskID, err.Error()))
	}
	t, err := task.FindOneIdAndExecutionWithDisplayStatus(taskID, nil)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := util.GetAPITaskFromTask(ctx, r.sc.GetURL(), *t)
	return apiTask, err
}

func (r *mutationResolver) ScheduleTasks(ctx context.Context, taskIds []string) ([]*restModel.APITask, error) {
	scheduledTasks := []*restModel.APITask{}
	scheduled, err := util.SetManyTasksScheduled(ctx, r.sc.GetURL(), true, taskIds...)
	if err != nil {
		return scheduledTasks, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Failed to schedule tasks : %s", err.Error()))
	}
	scheduledTasks = append(scheduledTasks, scheduled...)
	return scheduledTasks, nil
}

func (r *mutationResolver) SetTaskPriority(ctx context.Context, taskID string, priority int) (*restModel.APITask, error) {
	t, err := task.FindOneId(taskID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	authUser := gimlet.GetUser(ctx)
	if priority > evergreen.MaxTaskPriority {
		requiredPermission := gimlet.PermissionOpts{
			Resource:      t.Project,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionTasks,
			RequiredLevel: evergreen.TasksAdmin.Value,
		}
		isTaskAdmin := authUser.HasPermission(requiredPermission)
		if !isTaskAdmin {
			return nil, gqlError.Forbidden.Send(ctx, fmt.Sprintf("Insufficient access to set priority %v, can only set priority less than or equal to %v", priority, evergreen.MaxTaskPriority))
		}
	}
	if err = model.SetTaskPriority(*t, int64(priority), authUser.Username()); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error setting task priority %v: %v", taskID, err.Error()))
	}

	t, err = task.FindOneId(taskID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := util.GetAPITaskFromTask(ctx, r.sc.GetURL(), *t)
	return apiTask, err
}

func (r *mutationResolver) UnscheduleTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	scheduled, err := util.SetManyTasksScheduled(ctx, r.sc.GetURL(), false, taskID)
	if err != nil {
		return nil, err
	}
	if len(scheduled) == 0 {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find task: %s", taskID))
	}
	return scheduled[0], nil
}

func (r *mutationResolver) ClearMySubscriptions(ctx context.Context) (int, error) {
	usr := util.MustHaveUser(ctx)
	username := usr.Username()
	subs, err := event.FindSubscriptionsByOwner(username, event.OwnerTypePerson)
	if err != nil {
		return 0, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error retrieving subscriptions %s", err.Error()))
	}
	subIDs := util.RemoveGeneralSubscriptions(usr, subs)
	err = data.DeleteSubscriptions(username, subIDs)
	if err != nil {
		return 0, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error deleting subscriptions %s", err.Error()))
	}
	return len(subIDs), nil
}

func (r *mutationResolver) CreatePublicKey(ctx context.Context, publicKeyInput gqlModel.PublicKeyInput) ([]*restModel.APIPubKey, error) {
	err := util.SavePublicKey(ctx, publicKeyInput)
	if err != nil {
		return nil, err
	}
	myPublicKeys := util.GetMyPublicKeys(ctx)
	return myPublicKeys, nil
}

func (r *mutationResolver) RemovePublicKey(ctx context.Context, keyName string) ([]*restModel.APIPubKey, error) {
	if !util.DoesPublicKeyNameAlreadyExist(ctx, keyName) {
		return nil, gqlError.InputValidationError.Send(ctx, fmt.Sprintf("Error deleting public key. Provided key name, %s, does not exist.", keyName))
	}
	err := util.MustHaveUser(ctx).DeletePublicKey(keyName)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error deleting public key: %s", err.Error()))
	}
	myPublicKeys := util.GetMyPublicKeys(ctx)
	return myPublicKeys, nil
}

func (r *mutationResolver) SaveSubscription(ctx context.Context, subscription restModel.APISubscription) (bool, error) {
	usr := util.MustHaveUser(ctx)
	username := usr.Username()
	idType, id, err := util.GetResourceTypeAndIdFromSubscriptionSelectors(ctx, subscription.Selectors)
	if err != nil {
		return false, err
	}
	switch idType {
	case "task":
		t, taskErr := task.FindOneId(id)
		if taskErr != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", id, taskErr.Error()))
		}
		if t == nil {
			return false, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", id))
		}
	case "build":
		b, buildErr := build.FindOneId(id)
		if buildErr != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding build by id %s: %s", id, buildErr.Error()))
		}
		if b == nil {
			return false, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find build with id %s", id))
		}
	case "version":
		v, versionErr := model.VersionFindOneId(id)
		if versionErr != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding version by id %s: %s", id, versionErr.Error()))
		}
		if v == nil {
			return false, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find version with id %s", id))
		}
	case "project":
		p, projectErr := data.FindProjectById(id, false, false)
		if projectErr != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding project by id %s: %s", id, projectErr.Error()))
		}
		if p == nil {
			return false, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find project with id %s", id))
		}
	default:
		return false, gqlError.InputValidationError.Send(ctx, "Selectors do not indicate a target version, build, project, or task ID")
	}
	err = data.SaveSubscriptions(username, []restModel.APISubscription{subscription}, false)
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error saving subscription: %s", err.Error()))
	}
	return true, nil
}

func (r *mutationResolver) UpdatePublicKey(ctx context.Context, targetKeyName string, updateInfo gqlModel.PublicKeyInput) ([]*restModel.APIPubKey, error) {
	if !util.DoesPublicKeyNameAlreadyExist(ctx, targetKeyName) {
		return nil, gqlError.InputValidationError.Send(ctx, fmt.Sprintf("Error updating public key. The target key name, %s, does not exist.", targetKeyName))
	}
	if updateInfo.Name != targetKeyName && util.DoesPublicKeyNameAlreadyExist(ctx, updateInfo.Name) {
		return nil, gqlError.InputValidationError.Send(ctx, fmt.Sprintf("Error updating public key. The updated key name, %s, already exists.", targetKeyName))
	}
	err := util.VerifyPublicKey(ctx, updateInfo)
	if err != nil {
		return nil, err
	}
	usr := util.MustHaveUser(ctx)
	err = usr.UpdatePublicKey(targetKeyName, updateInfo.Name, updateInfo.Key)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error updating public key, %s: %s", targetKeyName, err.Error()))
	}
	myPublicKeys := util.GetMyPublicKeys(ctx)
	return myPublicKeys, nil
}

func (r *mutationResolver) UpdateUserSettings(ctx context.Context, userSettings *restModel.APIUserSettings) (bool, error) {
	usr := util.MustHaveUser(ctx)

	updatedUserSettings, err := restModel.UpdateUserSettings(ctx, usr, *userSettings)
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, err.Error())
	}
	err = data.UpdateSettings(usr, *updatedUserSettings)
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error saving userSettings : %s", err.Error()))
	}
	return true, nil
}

func (r *mutationResolver) RemoveItemFromCommitQueue(ctx context.Context, commitQueueID string, issue string) (*string, error) {
	result, err := data.CommitQueueRemoveItem(commitQueueID, issue, gimlet.GetUser(ctx).DisplayName())
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error removing item %s from commit queue %s: %s",
			issue, commitQueueID, err.Error()))
	}
	if result == nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't remove item %s from commit queue %s", issue, commitQueueID))
	}
	return &issue, nil
}

func (r *mutationResolver) RestartVersions(ctx context.Context, versionID string, abort bool, versionsToRestart []*model.VersionToRestart) ([]*restModel.APIVersion, error) {
	if len(versionsToRestart) == 0 {
		return nil, gqlError.InputValidationError.Send(ctx, "No versions provided. You must provide at least one version to restart")
	}
	modifications := model.VersionModification{
		Action:            evergreen.RestartAction,
		Abort:             abort,
		VersionsToRestart: versionsToRestart,
	}
	err := util.ModifyVersionHandler(ctx, versionID, modifications)
	if err != nil {
		return nil, err
	}
	versions := []*restModel.APIVersion{}
	for _, version := range versionsToRestart {
		if version.VersionId != nil {
			v, versionErr := model.VersionFindOneId(*version.VersionId)
			if versionErr != nil {
				return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding version by id %s: %s", *version.VersionId, versionErr.Error()))
			}
			if v == nil {
				return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find version with id %s", *version.VersionId))
			}
			apiVersion := restModel.APIVersion{}
			if err = apiVersion.BuildFromService(v); err != nil {
				return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building APIVersion from service for `%s`: %s", *version.VersionId, err.Error()))
			}
			versions = append(versions, &apiVersion)
		}
	}
	return versions, nil
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

type mutationResolver struct{ *Resolver }
