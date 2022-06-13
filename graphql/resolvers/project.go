package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	"github.com/evergreen-ci/evergreen/graphql/resolvers/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	werrors "github.com/pkg/errors"
)

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

func (r *mutationResolver) DeactivateStepbackTasks(ctx context.Context, projectID string) (bool, error) {
	usr := util.MustHaveUser(ctx)
	if err := task.DeactivateStepbackTasksForProject(projectID, usr.Username()); err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("deactivating current stepback tasks: %s", err.Error()))
	}
	return true, nil
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

func (r *projectResolver) IsFavorite(ctx context.Context, obj *restModel.APIProjectRef) (bool, error) {
	p, err := model.FindBranchProjectRef(*obj.Identifier)
	if err != nil || p == nil {
		return false, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project: %s : %s", *obj.Identifier, err))
	}
	usr := util.MustHaveUser(ctx)
	if utility.StringSliceContains(usr.FavoriteProjects, *obj.Identifier) {
		return true, nil
	}
	return false, nil
}

func (r *projectResolver) ValidDefaultLoggers(ctx context.Context, obj *restModel.APIProjectRef) ([]string, error) {
	return model.ValidDefaultLoggers, nil
}

func (r *projectSettingsResolver) Aliases(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APIProjectAlias, error) {
	return util.GetAPIAliasesForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

func (r *projectSettingsResolver) GithubWebhooksEnabled(ctx context.Context, obj *restModel.APIProjectSettings) (bool, error) {
	hook, err := model.FindGithubHook(utility.FromStringPtr(obj.ProjectRef.Owner), utility.FromStringPtr(obj.ProjectRef.Repo))
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Database error finding github hook for project '%s': %s", *obj.ProjectRef.Id, err.Error()))
	}
	return hook != nil, nil
}

func (r *projectSettingsResolver) Subscriptions(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APISubscription, error) {
	return util.GetAPISubscriptionsForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

func (r *projectSettingsResolver) Vars(ctx context.Context, obj *restModel.APIProjectSettings) (*restModel.APIProjectVars, error) {
	return util.GetRedactedAPIVarsForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

func (r *projectSubscriberResolver) Subscriber(ctx context.Context, obj *restModel.APISubscriber) (*gqlModel.Subscriber, error) {
	res := &gqlModel.Subscriber{}
	subscriberType := utility.FromStringPtr(obj.Type)

	switch subscriberType {
	case event.GithubPullRequestSubscriberType:
		sub := restModel.APIGithubPRSubscriber{}
		if err := mapstructure.Decode(obj.Target, &sub); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("problem converting %s subscriber: %s",
				event.GithubPullRequestSubscriberType, err.Error()))
		}
		res.GithubPRSubscriber = &sub
	case event.GithubCheckSubscriberType:
		sub := restModel.APIGithubCheckSubscriber{}
		if err := mapstructure.Decode(obj.Target, &sub); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("problem building %s subscriber from service: %s",
				event.GithubCheckSubscriberType, err.Error()))
		}
		res.GithubCheckSubscriber = &sub

	case event.EvergreenWebhookSubscriberType:
		sub := restModel.APIWebhookSubscriber{}
		if err := mapstructure.Decode(obj.Target, &sub); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("problem building %s subscriber from service: %s",
				event.EvergreenWebhookSubscriberType, err.Error()))
		}
		res.WebhookSubscriber = &sub

	case event.JIRAIssueSubscriberType:
		sub := &restModel.APIJIRAIssueSubscriber{}
		if err := mapstructure.Decode(obj.Target, &sub); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("problem building %s subscriber from service: %s",
				event.JIRAIssueSubscriberType, err.Error()))
		}
		res.JiraIssueSubscriber = sub
	case event.JIRACommentSubscriberType:
		res.JiraCommentSubscriber = obj.Target.(*string)
	case event.EmailSubscriberType:
		res.EmailSubscriber = obj.Target.(*string)
	case event.SlackSubscriberType:
		res.SlackSubscriber = obj.Target.(*string)
	case event.EnqueuePatchSubscriberType:
		// We don't store information in target for this case, so do nothing.
	default:
		return nil, werrors.Errorf("unknown subscriber type: '%s'", subscriberType)
	}

	return res, nil
}

func (r *projectVarsResolver) AdminOnlyVars(ctx context.Context, obj *restModel.APIProjectVars) ([]*string, error) {
	res := []*string{}
	for varAlias, isAdminOnly := range obj.AdminOnlyVars {
		if isAdminOnly {
			res = append(res, utility.ToStringPtr(varAlias))
		}
	}
	return res, nil
}

func (r *projectVarsResolver) PrivateVars(ctx context.Context, obj *restModel.APIProjectVars) ([]*string, error) {
	res := []*string{}
	for privateAlias, isPrivate := range obj.PrivateVars {
		if isPrivate {
			res = append(res, utility.ToStringPtr(privateAlias))
		}
	}
	return res, nil
}

func (r *queryResolver) GithubProjectConflicts(ctx context.Context, projectID string) (*model.GithubProjectConflicts, error) {
	pRef, err := model.FindMergedProjectRef(projectID, "", false)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error getting project: %v", err.Error()))
	}
	if pRef == nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("project '%s' not found", projectID))
	}

	conflicts, err := pRef.GetGithubProjectConflicts()
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error getting project conflicts: %v", err.Error()))
	}
	return &conflicts, nil
}

func (r *queryResolver) Project(ctx context.Context, projectID string) (*restModel.APIProjectRef, error) {
	project, err := data.FindProjectById(projectID, true, false)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding project by id %s: %s", projectID, err.Error()))
	}
	apiProjectRef := restModel.APIProjectRef{}
	err = apiProjectRef.BuildFromService(project)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error building APIProject from service: %s", err.Error()))
	}
	return &apiProjectRef, nil
}

func (r *queryResolver) Projects(ctx context.Context) ([]*gqlModel.GroupedProjects, error) {
	allProjects, err := model.FindAllMergedTrackedProjectRefs()
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, err.Error())
	}
	// We have to iterate over the merged project refs to verify if they are enabled
	enabledProjects := []model.ProjectRef{}
	for _, p := range allProjects {
		if p.IsEnabled() {
			enabledProjects = append(enabledProjects, p)
		}
	}
	groupedProjects, err := util.GroupProjects(enabledProjects, false)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error grouping project: %s", err.Error()))
	}
	return groupedProjects, nil
}

func (r *queryResolver) ProjectEvents(ctx context.Context, identifier string, limit *int, before *time.Time) (*gqlModel.ProjectEvents, error) {
	timestamp := time.Now()
	if before != nil {
		timestamp = *before
	}
	events, err := data.GetProjectEventLog(identifier, timestamp, utility.FromIntPtr(limit))
	res := &gqlModel.ProjectEvents{
		EventLogEntries: util.GetPointerEventList(events),
		Count:           len(events),
	}
	return res, err
}

func (r *queryResolver) ProjectSettings(ctx context.Context, identifier string) (*restModel.APIProjectSettings, error) {
	projectRef, err := model.FindBranchProjectRef(identifier)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error looking in project collection: %s", err.Error()))
	}
	if projectRef == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, "project doesn't exist")
	}

	res := &restModel.APIProjectSettings{
		ProjectRef: restModel.APIProjectRef{},
	}
	if err = res.ProjectRef.BuildFromService(projectRef); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error building APIProjectRef from service: %s", err.Error()))
	}
	if !projectRef.UseRepoSettings() {
		// Default values so the UI understands what to do with nil values.
		res.ProjectRef.DefaultUnsetBooleans()
	}
	return res, nil
}

func (r *queryResolver) RepoEvents(ctx context.Context, id string, limit *int, before *time.Time) (*gqlModel.ProjectEvents, error) {
	timestamp := time.Now()
	if before != nil {
		timestamp = *before
	}
	events, err := data.GetEventsById(id, timestamp, utility.FromIntPtr(limit))
	res := &gqlModel.ProjectEvents{
		EventLogEntries: util.GetPointerEventList(events),
		Count:           len(events),
	}
	return res, err
}

func (r *queryResolver) RepoSettings(ctx context.Context, id string) (*restModel.APIProjectSettings, error) {
	repoRef, err := model.FindOneRepoRef(id)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error looking in repo collection: %s", err.Error()))
	}
	if repoRef == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, "repo doesn't exist")
	}

	res := &restModel.APIProjectSettings{
		ProjectRef: restModel.APIProjectRef{},
	}
	if err = res.ProjectRef.BuildFromService(repoRef.ProjectRef); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error building APIProjectRef from service: %s", err.Error()))
	}

	// Default values so the UI understands what to do with nil values.
	res.ProjectRef.DefaultUnsetBooleans()
	return res, nil
}

func (r *queryResolver) ViewableProjectRefs(ctx context.Context) ([]*gqlModel.GroupedProjects, error) {
	usr := util.MustHaveUser(ctx)
	projectIds, err := usr.GetViewableProjectSettings()
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error getting viewable projects for '%s': '%s'", usr.DispName, err.Error()))
	}

	projects, err := model.FindProjectRefsByIds(projectIds...)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting projects: %v", err.Error()))
	}

	groupedProjects, err := util.GroupProjects(projects, true)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error grouping project: %s", err.Error()))
	}
	return groupedProjects, nil
}

func (r *repoRefResolver) ValidDefaultLoggers(ctx context.Context, obj *restModel.APIProjectRef) ([]string, error) {
	return model.ValidDefaultLoggers, nil
}

func (r *repoSettingsResolver) Aliases(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APIProjectAlias, error) {
	return util.GetAPIAliasesForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

func (r *repoSettingsResolver) GithubWebhooksEnabled(ctx context.Context, obj *restModel.APIProjectSettings) (bool, error) {
	hook, err := model.FindGithubHook(utility.FromStringPtr(obj.ProjectRef.Owner), utility.FromStringPtr(obj.ProjectRef.Repo))
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Database error finding github hook for project '%s': %s", *obj.ProjectRef.Id, err.Error()))
	}
	return hook != nil, nil
}

func (r *repoSettingsResolver) Subscriptions(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APISubscription, error) {
	return util.GetAPISubscriptionsForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

func (r *repoSettingsResolver) Vars(ctx context.Context, obj *restModel.APIProjectSettings) (*restModel.APIProjectVars, error) {
	return util.GetRedactedAPIVarsForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

// Project returns generated.ProjectResolver implementation.
func (r *Resolver) Project() generated.ProjectResolver { return &projectResolver{r} }

// ProjectSettings returns generated.ProjectSettingsResolver implementation.
func (r *Resolver) ProjectSettings() generated.ProjectSettingsResolver {
	return &projectSettingsResolver{r}
}

// ProjectSubscriber returns generated.ProjectSubscriberResolver implementation.
func (r *Resolver) ProjectSubscriber() generated.ProjectSubscriberResolver {
	return &projectSubscriberResolver{r}
}

// ProjectVars returns generated.ProjectVarsResolver implementation.
func (r *Resolver) ProjectVars() generated.ProjectVarsResolver { return &projectVarsResolver{r} }

// RepoRef returns generated.RepoRefResolver implementation.
func (r *Resolver) RepoRef() generated.RepoRefResolver { return &repoRefResolver{r} }

// RepoSettings returns generated.RepoSettingsResolver implementation.
func (r *Resolver) RepoSettings() generated.RepoSettingsResolver { return &repoSettingsResolver{r} }

type projectResolver struct{ *Resolver }
type projectSettingsResolver struct{ *Resolver }
type projectSubscriberResolver struct{ *Resolver }
type projectVarsResolver struct{ *Resolver }
type repoRefResolver struct{ *Resolver }
type repoSettingsResolver struct{ *Resolver }
