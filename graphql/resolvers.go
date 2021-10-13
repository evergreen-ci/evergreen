package graphql

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/api"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

type Resolver struct {
	sc data.Connector
}

func (r *Resolver) Mutation() MutationResolver {
	return &mutationResolver{r}
}
func (r *Resolver) Patch() PatchResolver {
	return &patchResolver{r}
}
func (r *Resolver) Query() QueryResolver {
	return &queryResolver{r}
}
func (r *Resolver) Task() TaskResolver {
	return &taskResolver{r}
}
func (r *Resolver) Host() HostResolver {
	return &hostResolver{r}
}
func (r *Resolver) Volume() VolumeResolver {
	return &volumeResolver{r}
}
func (r *Resolver) TaskQueueItem() TaskQueueItemResolver {
	return &taskQueueItemResolver{r}
}
func (r *Resolver) User() UserResolver {
	return &userResolver{r}
}
func (r *Resolver) Project() ProjectResolver {
	return &projectResolver{r}
}
func (r *Resolver) Annotation() AnnotationResolver {
	return &annotationResolver{r}
}
func (r *Resolver) TaskLogs() TaskLogsResolver {
	return &taskLogsResolver{r}
}

func (r *Resolver) ProjectSettings() ProjectSettingsResolver {
	return &projectSettingsResolver{r}
}

func (r *Resolver) ProjectSubscriber() ProjectSubscriberResolver {
	return &projectSubscriberResolver{r}
}

// IssueLink returns IssueLinkResolver implementation.
func (r *Resolver) IssueLink() IssueLinkResolver {
	return &issueLinkResolver{r}
}

func (r *Resolver) ProjectVars() ProjectVarsResolver {
	return &projectVarsResolver{r}
}

type hostResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }
type taskQueueItemResolver struct{ *Resolver }
type volumeResolver struct{ *Resolver }
type userResolver struct{ *Resolver }
type projectResolver struct{ *Resolver }
type annotationResolver struct{ *Resolver }
type issueLinkResolver struct{ *Resolver }
type projectSettingsResolver struct{ *Resolver }
type projectSubscriberResolver struct{ *Resolver }
type projectVarsResolver struct{ *Resolver }
type taskLogsResolver struct{ *Resolver }

func (r *hostResolver) DistroID(ctx context.Context, obj *restModel.APIHost) (*string, error) {
	return obj.Distro.Id, nil
}

func (r *hostResolver) HomeVolume(ctx context.Context, obj *restModel.APIHost) (*restModel.APIVolume, error) {
	if obj.HomeVolumeID != nil && *obj.HomeVolumeID != "" {
		volId := *obj.HomeVolumeID
		volume, err := r.sc.FindVolumeById(volId)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting volume %s: %s", volId, err.Error()))
		}
		apiVolume := &restModel.APIVolume{}
		err = apiVolume.BuildFromService(volume)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building volume '%s' from service: %s", volId, err.Error()))
		}
		return apiVolume, nil
	}
	return nil, nil
}

func (r *hostResolver) Uptime(ctx context.Context, obj *restModel.APIHost) (*time.Time, error) {
	return obj.CreationTime, nil
}

func (r *hostResolver) Elapsed(ctx context.Context, obj *restModel.APIHost) (*time.Time, error) {
	return obj.RunningTask.StartTime, nil
}

func (r *hostResolver) Volumes(ctx context.Context, obj *restModel.APIHost) ([]*restModel.APIVolume, error) {
	volumes := make([]*restModel.APIVolume, 0, len(obj.AttachedVolumeIDs))
	for _, volId := range obj.AttachedVolumeIDs {
		volume, err := r.sc.FindVolumeById(volId)
		if err != nil {
			return volumes, InternalServerError.Send(ctx, fmt.Sprintf("Error getting volume %s", volId))
		}
		if volume == nil {
			continue
		}
		apiVolume := &restModel.APIVolume{}
		err = apiVolume.BuildFromService(volume)
		if err != nil {
			return nil, InternalServerError.Send(ctx, errors.Wrapf(err, "error building volume '%s' from service", volId).Error())
		}
		volumes = append(volumes, apiVolume)
	}

	return volumes, nil
}

func (r *volumeResolver) Host(ctx context.Context, obj *restModel.APIVolume) (*restModel.APIHost, error) {
	if obj.HostID == nil || *obj.HostID == "" {
		return nil, nil
	}
	host, err := r.sc.FindHostById(*obj.HostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding host %s: %s", *obj.HostID, err.Error()))
	}
	if host == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find host %s", *obj.HostID))
	}
	apiHost := restModel.APIHost{}
	err = apiHost.BuildFromService(host)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost %s from service: %s", host.Id, err))
	}
	return &apiHost, nil
}

func (r *queryResolver) MyPublicKeys(ctx context.Context) ([]*restModel.APIPubKey, error) {
	publicKeys := getMyPublicKeys(ctx)
	return publicKeys, nil
}

func (r *taskResolver) Project(ctx context.Context, obj *restModel.APITask) (*restModel.APIProjectRef, error) {
	pRef, err := r.sc.FindProjectById(*obj.ProjectId, true)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding project ref for project %s: %s", *obj.ProjectId, err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find a ProjectRef for project %s", *obj.ProjectId))
	}
	apiProjectRef := restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(pRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APIProject from service: %s", err.Error()))
	}

	return &apiProjectRef, nil
}

func (r *taskResolver) AbortInfo(ctx context.Context, at *restModel.APITask) (*AbortInfo, error) {
	if !at.Aborted {
		return nil, nil
	}

	info := AbortInfo{
		User:       at.AbortInfo.User,
		TaskID:     at.AbortInfo.TaskID,
		NewVersion: at.AbortInfo.NewVersion,
		PrClosed:   at.AbortInfo.PRClosed,
	}

	if len(at.AbortInfo.TaskID) > 0 {
		abortedTask, err := task.FindOneId(at.AbortInfo.TaskID)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Problem getting aborted task %s: %s", *at.Id, err.Error()))
		}
		if abortedTask == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find aborted task %s: %s", at.AbortInfo.TaskID, err.Error()))
		}
		abortedTaskBuild, err := build.FindOneId(abortedTask.BuildId)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Problem getting build for aborted task %s: %s", abortedTask.BuildId, err.Error()))
		}
		if abortedTaskBuild == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find build %s for aborted task: %s", abortedTask.BuildId, err.Error()))
		}
		info.TaskDisplayName = abortedTask.DisplayName
		info.BuildVariantDisplayName = abortedTaskBuild.DisplayName
	}

	return &info, nil
}

func (r *taskResolver) ReliesOn(ctx context.Context, at *restModel.APITask) ([]*Dependency, error) {
	dependencies := []*Dependency{}
	if len(at.DependsOn) == 0 {
		return dependencies, nil
	}
	depIds := []string{}
	for _, dep := range at.DependsOn {
		depIds = append(depIds, dep.TaskId)
	}

	dependencyTasks, err := task.Find(task.ByIds(depIds).WithFields(task.DisplayNameKey, task.StatusKey,
		task.ActivatedKey, task.BuildVariantKey, task.DetailsKey, task.DependsOnKey))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Cannot find dependency tasks for task %s: %s", *at.Id, err.Error()))
	}

	taskMap := map[string]*task.Task{}
	for i := range dependencyTasks {
		taskMap[dependencyTasks[i].Id] = &dependencyTasks[i]
	}

	i, err := at.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *at.Id, err.Error()))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *at.Id))
	}

	for _, dep := range at.DependsOn {
		depTask, ok := taskMap[dep.TaskId]
		if !ok {
			continue
		}
		var metStatus MetStatus
		if !depTask.IsFinished() {
			metStatus = "PENDING"
		} else if t.SatisfiesDependency(depTask) {
			metStatus = "MET"
		} else {
			metStatus = "UNMET"
		}
		var requiredStatus RequiredStatus
		switch dep.Status {
		case model.AllStatuses:
			requiredStatus = "MUST_FINISH"
			break
		case evergreen.TaskFailed:
			requiredStatus = "MUST_FAIL"
			break
		default:
			requiredStatus = "MUST_SUCCEED"
		}

		dependency := Dependency{
			Name:           depTask.DisplayName,
			BuildVariant:   depTask.BuildVariant,
			MetStatus:      metStatus,
			RequiredStatus: requiredStatus,
			UILink:         fmt.Sprintf("/task/%s", depTask.Id),
		}

		dependencies = append(dependencies, &dependency)
	}
	return dependencies, nil
}

func (r *taskResolver) DependsOn(ctx context.Context, at *restModel.APITask) ([]*Dependency, error) {
	dependencies := []*Dependency{}
	if len(at.DependsOn) == 0 {
		return nil, nil
	}
	depIds := []string{}
	for _, dep := range at.DependsOn {
		depIds = append(depIds, dep.TaskId)
	}

	dependencyTasks, err := task.Find(task.ByIds(depIds).WithFields(task.DisplayNameKey, task.StatusKey,
		task.ActivatedKey, task.BuildVariantKey, task.DetailsKey, task.DependsOnKey))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Cannot find dependency tasks for task %s: %s", *at.Id, err.Error()))
	}

	taskMap := map[string]*task.Task{}
	for i := range dependencyTasks {
		taskMap[dependencyTasks[i].Id] = &dependencyTasks[i]
	}

	i, err := at.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *at.Id, err.Error()))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *at.Id))
	}

	for _, dep := range at.DependsOn {
		depTask, ok := taskMap[dep.TaskId]
		if !ok {
			continue
		}
		var metStatus MetStatus
		if !depTask.IsFinished() {
			metStatus = "PENDING"
		} else if t.SatisfiesDependency(depTask) {
			metStatus = "MET"
		} else {
			metStatus = "UNMET"
		}
		var requiredStatus RequiredStatus
		switch dep.Status {
		case model.AllStatuses:
			requiredStatus = "MUST_FINISH"
		case evergreen.TaskFailed:
			requiredStatus = "MUST_FAIL"
		default:
			requiredStatus = "MUST_SUCCEED"
		}

		dependency := Dependency{
			Name:           depTask.DisplayName,
			BuildVariant:   depTask.BuildVariant,
			MetStatus:      metStatus,
			RequiredStatus: requiredStatus,
			TaskID:         dep.TaskId,
		}

		dependencies = append(dependencies, &dependency)
	}
	return dependencies, nil
}

func (r *taskResolver) CanOverrideDependencies(ctx context.Context, at *restModel.APITask) (bool, error) {
	currentUser := MustHaveUser(ctx)
	if at.OverrideDependencies {
		return false, nil
	}
	// if the task is not the latest execution of the task, it can't be overridden
	if at.Archived {
		return false, nil
	}
	requiredPermission := gimlet.PermissionOpts{
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionTasks,
		RequiredLevel: evergreen.TasksAdmin.Value,
		Resource:      *at.ProjectId,
	}
	if len(at.DependsOn) > 0 && currentUser.HasPermission(requiredPermission) {
		return true, nil
	}
	return false, nil
}

func (r *projectResolver) IsFavorite(ctx context.Context, at *restModel.APIProjectRef) (bool, error) {
	p, err := model.FindBranchProjectRef(*at.Identifier)
	if err != nil || p == nil {
		return false, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project: %s : %s", *at.Identifier, err))
	}
	usr := MustHaveUser(ctx)
	if utility.StringSliceContains(usr.FavoriteProjects, *at.Identifier) {
		return true, nil
	}
	return false, nil
}

func (r *projectSettingsResolver) GithubWebhooksEnabled(ctx context.Context, a *restModel.APIProjectSettings) (bool, error) {
	hook, err := model.FindGithubHook(utility.FromStringPtr(a.ProjectRef.Owner), utility.FromStringPtr(a.ProjectRef.Repo))
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("Database error finding github hook for project '%s': %s", *a.ProjectRef.Id, err.Error()))
	}
	return hook != nil, nil
}

func (r *projectSettingsResolver) Vars(ctx context.Context, a *restModel.APIProjectSettings) (*restModel.APIProjectVars, error) {
	vars, err := model.FindOneProjectVars(utility.FromStringPtr(a.ProjectRef.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding project vars for '%s': %s", *a.ProjectRef.Id, err.Error()))
	}
	if vars == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("vars for '%s' don't exist", *a.ProjectRef.Id))
	}
	res := &restModel.APIProjectVars{}
	if err = res.BuildFromService(vars); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem building APIProjectVars from service: %s", err.Error()))
	}
	return res, nil
}

func (r *projectSettingsResolver) Aliases(ctx context.Context, a *restModel.APIProjectSettings) ([]*restModel.APIProjectAlias, error) {
	aliases, err := model.FindAliasesForProject(utility.FromStringPtr(a.ProjectRef.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding aliases for project: %s", err.Error()))
	}
	res := []*restModel.APIProjectAlias{}
	for _, alias := range aliases {
		apiAlias := restModel.APIProjectAlias{}
		if err = apiAlias.BuildFromService(alias); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem building APIPProjectAlias %s from service: %s",
				alias.Alias, err.Error()))
		}
		res = append(res, &apiAlias)
	}
	return res, nil
}

func (r *projectVarsResolver) PrivateVars(ctx context.Context, a *restModel.APIProjectVars) ([]*string, error) {
	res := []*string{}
	for privateAlias, isPrivate := range a.PrivateVars {
		if isPrivate {
			res = append(res, utility.ToStringPtr(privateAlias))
		}
	}
	return res, nil
}

func (r *projectSettingsResolver) Subscriptions(ctx context.Context, a *restModel.APIProjectSettings) ([]*restModel.APISubscription, error) {
	subscriptions, err := event.FindSubscriptionsByOwner(utility.FromStringPtr(a.ProjectRef.Id), event.OwnerTypeProject)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding subscription for project: %s", err.Error()))
	}

	res := []*restModel.APISubscription{}
	for _, sub := range subscriptions {
		apiSubscription := restModel.APISubscription{}
		if err = apiSubscription.BuildFromService(sub); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem building APIPProjectSubscription %s from service: %s",
				sub.ID, err.Error()))
		}
		res = append(res, &apiSubscription)
	}
	return res, nil
}

func (r *projectSubscriberResolver) Subscriber(ctx context.Context, a *restModel.APISubscriber) (*Subscriber, error) {
	res := &Subscriber{}
	subscriberType := utility.FromStringPtr(a.Type)

	switch subscriberType {
	case event.GithubPullRequestSubscriberType:
		sub := restModel.APIGithubPRSubscriber{}
		if err := mapstructure.Decode(a.Target, &sub); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem converting %s subscriber: %s",
				event.GithubPullRequestSubscriberType, err.Error()))
		}
		res.GithubPRSubscriber = &sub
	case event.GithubCheckSubscriberType:
		sub := restModel.APIGithubCheckSubscriber{}
		if err := mapstructure.Decode(a.Target, &sub); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem building %s subscriber from service: %s",
				event.GithubCheckSubscriberType, err.Error()))
		}
		res.GithubCheckSubscriber = &sub

	case event.EvergreenWebhookSubscriberType:
		sub := restModel.APIWebhookSubscriber{}
		if err := mapstructure.Decode(a.Target, &sub); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem building %s subscriber from service: %s",
				event.EvergreenWebhookSubscriberType, err.Error()))
		}
		res.WebhookSubscriber = &sub

	case event.JIRAIssueSubscriberType:
		sub := &restModel.APIJIRAIssueSubscriber{}
		if err := mapstructure.Decode(a.Target, &sub); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem building %s subscriber from service: %s",
				event.JIRAIssueSubscriberType, err.Error()))
		}
		res.JiraIssueSubscriber = sub
	case event.JIRACommentSubscriberType:
		res.JiraCommentSubscriber = a.Target.(*string)
	case event.EmailSubscriberType:
		res.EmailSubscriber = a.Target.(*string)
	case event.SlackSubscriberType:
		res.SlackSubscriber = a.Target.(*string)
	case event.EnqueuePatchSubscriberType:
		// We don't store information in target for this case, so do nothing.
	default:
		return nil, errors.Errorf("unknown subscriber type: '%s'", subscriberType)
	}

	return res, nil
}

func (r *mutationResolver) AddFavoriteProject(ctx context.Context, identifier string) (*restModel.APIProjectRef, error) {
	p, err := model.FindBranchProjectRef(identifier)
	if err != nil || p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("could not find project '%s'", identifier))
	}

	usr := MustHaveUser(ctx)

	err = usr.AddFavoritedProject(identifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	apiProjectRef := restModel.APIProjectRef{}
	err = apiProjectRef.BuildFromService(p)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error building APIProjectRef from service: %s", err.Error()))
	}
	return &apiProjectRef, nil
}

func (r *mutationResolver) RemoveFavoriteProject(ctx context.Context, identifier string) (*restModel.APIProjectRef, error) {
	p, err := model.FindBranchProjectRef(identifier)
	if err != nil || p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project: %s", identifier))
	}

	usr := MustHaveUser(ctx)

	err = usr.RemoveFavoriteProject(identifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error removing project : %s : %s", identifier, err))
	}
	apiProjectRef := restModel.APIProjectRef{}
	err = apiProjectRef.BuildFromService(p)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error building APIProjectRef from service: %s", err.Error()))
	}
	return &apiProjectRef, nil
}

func (r *mutationResolver) SpawnVolume(ctx context.Context, spawnVolumeInput SpawnVolumeInput) (bool, error) {
	err := validateVolumeExpirationInput(ctx, spawnVolumeInput.Expiration, spawnVolumeInput.NoExpiration)
	if err != nil {
		return false, err
	}
	success, _, gqlErr, err, vol := RequestNewVolume(ctx, GetVolumeFromSpawnVolumeInput(spawnVolumeInput))
	if err != nil {
		return false, gqlErr.Send(ctx, err.Error())
	}
	if vol == nil {
		return false, InternalServerError.Send(ctx, "Unable to create volume")
	}
	errorTemplate := "Volume %s has been created but an error occurred."
	var additionalOptions restModel.VolumeModifyOptions
	if spawnVolumeInput.Expiration != nil {
		var newExpiration time.Time
		newExpiration, err = restModel.FromTimePtr(spawnVolumeInput.Expiration)
		if err != nil {
			return false, gqlErr.Send(ctx, errors.Wrapf(err, errorTemplate, vol.ID).Error())
		}
		additionalOptions.Expiration = newExpiration
	} else if spawnVolumeInput.NoExpiration != nil && *spawnVolumeInput.NoExpiration == true {
		// this value should only ever be true or nil
		additionalOptions.NoExpiration = true
	}
	err = applyVolumeOptions(ctx, *vol, additionalOptions)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("Unable to apply expiration options to volume %s: %s", vol.ID, err.Error()))
	}
	if spawnVolumeInput.Host != nil {
		_, _, gqlErr, err := AttachVolume(ctx, vol.ID, *spawnVolumeInput.Host)
		if err != nil {
			return false, gqlErr.Send(ctx, errors.Wrapf(err, errorTemplate, vol.ID).Error())
		}
	}

	return success, nil
}

func (r *mutationResolver) UpdateVolume(ctx context.Context, updateVolumeInput UpdateVolumeInput) (bool, error) {
	volume, err := r.sc.FindVolumeById(updateVolumeInput.VolumeID)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("Error finding volume by id %s: %s", updateVolumeInput.VolumeID, err.Error()))
	}
	if volume == nil {
		return false, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find volume %s", volume.ID))
	}
	err = validateVolumeExpirationInput(ctx, updateVolumeInput.Expiration, updateVolumeInput.NoExpiration)
	if err != nil {
		return false, err
	}
	err = validateVolumeName(ctx, updateVolumeInput.Name)
	if err != nil {
		return false, err
	}
	var updateOptions restModel.VolumeModifyOptions
	if updateVolumeInput.NoExpiration != nil {
		if *updateVolumeInput.NoExpiration == true {
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
			return false, InternalServerError.Send(ctx, fmt.Sprintf("Error parsing time %s", err))
		}
		updateOptions.Expiration = newExpiration
	}
	if updateVolumeInput.Name != nil {
		updateOptions.NewName = *updateVolumeInput.Name
	}
	err = applyVolumeOptions(ctx, *volume, updateOptions)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("Unable to update volume %s: %s", volume.ID, err.Error()))
	}

	return true, nil
}

func (r *mutationResolver) SpawnHost(ctx context.Context, spawnHostInput *SpawnHostInput) (*restModel.APIHost, error) {
	usr := MustHaveUser(ctx)
	if spawnHostInput.SavePublicKey {
		if err := savePublicKey(ctx, *spawnHostInput.PublicKey); err != nil {
			return nil, err
		}
	}
	dist, err := distro.FindByID(spawnHostInput.DistroID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while trying to find distro with id: %s, err:  `%s`", spawnHostInput.DistroID, err))
	}
	if dist == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find Distro with id: %s", spawnHostInput.DistroID))
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
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error occurred finding task %s: %s", *spawnHostInput.TaskID, err.Error()))
		}
	}

	if utility.FromBoolPtr(spawnHostInput.UseProjectSetupScript) {
		if t == nil {
			return nil, ResourceNotFound.Send(ctx, "A valid task id must be supplied when useProjectSetupScript is set to true")
		}
		options.UseProjectSetupScript = *spawnHostInput.UseProjectSetupScript
	}
	if utility.FromBoolPtr(spawnHostInput.TaskSync) {
		if t == nil {
			return nil, ResourceNotFound.Send(ctx, "A valid task id must be supplied when taskSync is set to true")
		}
		options.TaskSync = *spawnHostInput.TaskSync
	}
	hc := &data.DBConnector{}

	if utility.FromBoolPtr(spawnHostInput.SpawnHostsStartedByTask) {
		if t == nil {
			return nil, ResourceNotFound.Send(ctx, "A valid task id must be supplied when SpawnHostsStartedByTask is set to true")
		}
		if err = hc.CreateHostsFromTask(t, *usr, spawnHostInput.PublicKey.Key); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error spawning hosts from task: %s : %s", *spawnHostInput.TaskID, err))
		}
	}

	spawnHost, err := hc.NewIntentHost(ctx, options, usr, evergreen.GetEnvironment().Settings())
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error spawning host: %s", err))
	}
	if spawnHost == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("An error occurred Spawn host is nil"))
	}
	apiHost := restModel.APIHost{}
	if err := apiHost.BuildFromService(spawnHost); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
	}
	return &apiHost, nil
}

func (r *mutationResolver) EditSpawnHost(ctx context.Context, editSpawnHostInput *EditSpawnHostInput) (*restModel.APIHost, error) {
	var v *host.Volume
	usr := MustHaveUser(ctx)
	h, err := host.FindOneByIdOrTag(editSpawnHostInput.HostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding host by id: %s", err))
	}

	if !CanUpdateSpawnHost(h, usr) {
		return nil, Forbidden.Send(ctx, "You are not authorized to modify this host")
	}

	opts := host.HostModifyOptions{}
	if editSpawnHostInput.DisplayName != nil {
		opts.NewName = *editSpawnHostInput.DisplayName
	}
	if editSpawnHostInput.NoExpiration != nil {
		opts.NoExpiration = editSpawnHostInput.NoExpiration
	}
	if editSpawnHostInput.Expiration != nil {
		err = h.SetExpirationTime(*editSpawnHostInput.Expiration)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while modifying spawnhost expiration time: %s", err))
		}
	}
	if editSpawnHostInput.InstanceType != nil {
		var config *evergreen.Settings
		config, err = evergreen.GetConfig()
		if err != nil {
			return nil, InternalServerError.Send(ctx, "unable to retrieve server config")
		}
		allowedTypes := config.Providers.AWS.AllowedInstanceTypes

		err = cloud.CheckInstanceTypeValid(ctx, h.Distro, *editSpawnHostInput.InstanceType, allowedTypes)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error validating instance type: %s", err))
		}
		opts.InstanceType = *editSpawnHostInput.InstanceType
	}
	if editSpawnHostInput.AddedInstanceTags != nil || editSpawnHostInput.DeletedInstanceTags != nil {
		addedTags := []host.Tag{}
		deletedTags := []string{}
		for _, tag := range editSpawnHostInput.AddedInstanceTags {
			tag.CanBeModified = true
			addedTags = append(addedTags, *tag)
		}
		for _, tag := range editSpawnHostInput.DeletedInstanceTags {
			deletedTags = append(deletedTags, tag.Key)
		}
		opts.AddInstanceTags = addedTags
		opts.DeleteInstanceTags = deletedTags
	}
	if editSpawnHostInput.Volume != nil {
		v, err = r.sc.FindVolumeById(*editSpawnHostInput.Volume)
		if err != nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Error finding requested volume id: %s", err))
		}
		if v.AvailabilityZone != h.Zone {
			return nil, InputValidationError.Send(ctx, fmt.Sprintf("Error mounting volume to spawn host, They must be in the same availability zone."))
		}
		opts.AttachVolume = *editSpawnHostInput.Volume
	}
	if editSpawnHostInput.PublicKey != nil {
		if utility.FromBoolPtr(editSpawnHostInput.SavePublicKey) {
			if err = savePublicKey(ctx, *editSpawnHostInput.PublicKey); err != nil {
				return nil, err
			}
		}
		opts.AddKey = editSpawnHostInput.PublicKey.Key
		if opts.AddKey == "" {
			opts.AddKey, err = r.sc.GetPublicKey(usr, editSpawnHostInput.PublicKey.Name)
			if err != nil {
				return nil, InputValidationError.Send(ctx, fmt.Sprintf("No matching key found for name '%s'", editSpawnHostInput.PublicKey.Name))
			}
		}
	}
	if err = cloud.ModifySpawnHost(ctx, evergreen.GetEnvironment(), h, opts); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error modifying spawn host: %s", err))
	}
	if editSpawnHostInput.ServicePassword != nil {
		_, _, err = UpdateHostPassword(ctx, evergreen.GetEnvironment(), h, usr, *editSpawnHostInput.ServicePassword, nil)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error setting spawn host password: %s", err))
		}
	}

	apiHost := restModel.APIHost{}
	err = apiHost.BuildFromService(h)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
	}
	return &apiHost, nil
}

func (r *mutationResolver) UpdateSpawnHostStatus(ctx context.Context, hostID string, action SpawnHostStatusActions) (*restModel.APIHost, error) {
	host, err := host.FindOneByIdOrTag(hostID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Error finding host by id: %s", err))
	}
	usr := MustHaveUser(ctx)
	env := evergreen.GetEnvironment()

	if !CanUpdateSpawnHost(host, usr) {
		return nil, Forbidden.Send(ctx, "You are not authorized to modify this host")
	}

	switch action {
	case SpawnHostStatusActionsStart:
		h, httpStatus, err := StartSpawnHost(ctx, env, host, usr, nil)
		if err != nil {
			return nil, mapHTTPStatusToGqlError(ctx, httpStatus, err)
		}
		apiHost := restModel.APIHost{}
		err = apiHost.BuildFromService(h)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
		}
		return &apiHost, nil
	case SpawnHostStatusActionsStop:
		h, httpStatus, err := StopSpawnHost(ctx, env, host, usr, nil)
		if err != nil {
			return nil, mapHTTPStatusToGqlError(ctx, httpStatus, err)
		}
		apiHost := restModel.APIHost{}
		err = apiHost.BuildFromService(h)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
		}
		return &apiHost, nil
	case SpawnHostStatusActionsTerminate:
		h, httpStatus, err := TerminateSpawnHost(ctx, env, host, usr, nil)
		if err != nil {
			return nil, mapHTTPStatusToGqlError(ctx, httpStatus, err)
		}
		apiHost := restModel.APIHost{}
		err = apiHost.BuildFromService(h)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
		}
		return &apiHost, nil
	default:
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find matching status for action : %s", action))
	}

}

type queryResolver struct{ *Resolver }

func (r *queryResolver) Hosts(ctx context.Context, hostID *string, distroID *string, currentTaskID *string, statuses []string, startedBy *string, sortBy *HostSortBy, sortDir *SortDirection, page *int, limit *int) (*HostsResponse, error) {
	hostIDParam := ""
	if hostID != nil {
		hostIDParam = *hostID
	}
	distroParam := ""
	if distroID != nil {
		distroParam = *distroID
	}
	currentTaskParam := ""
	if currentTaskID != nil {
		currentTaskParam = *currentTaskID
	}
	startedByParam := ""
	if startedBy != nil {
		startedByParam = *startedBy
	}
	sorter := host.StatusKey
	if sortBy != nil {
		switch *sortBy {
		case HostSortByCurrentTask:
			sorter = host.RunningTaskKey
			break
		case HostSortByDistro:
			sorter = host.DistroKey
			break
		case HostSortByElapsed:
			sorter = "task_full.start_time"
			break
		case HostSortByID:
			sorter = host.IdKey
			break
		case HostSortByIDLeTime:
			sorter = host.TotalIdleTimeKey
			break
		case HostSortByOwner:
			sorter = host.StartedByKey
			break
		case HostSortByStatus:
			sorter = host.StatusKey
			break
		case HostSortByUptime:
			sorter = host.CreateTimeKey
			break
		default:
			sorter = host.StatusKey
			break
		}

	}
	sortDirParam := 1
	if *sortDir == SortDirectionDesc {
		sortDirParam = -1
	}
	pageParam := 0
	if page != nil {
		pageParam = *page
	}
	limitParam := 0
	if limit != nil {
		limitParam = *limit
	}

	hosts, filteredHostsCount, totalHostsCount, err := host.GetPaginatedRunningHosts(hostIDParam, distroParam, currentTaskParam, statuses, startedByParam, sorter, sortDirParam, pageParam, limitParam)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting hosts: %s", err.Error()))
	}

	apiHosts := []*restModel.APIHost{}

	for _, host := range hosts {
		apiHost := restModel.APIHost{}

		err = apiHost.BuildFromService(host)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building API Host from Service: %s", err.Error()))
		}

		if host.RunningTask != "" {
			// Add the task information to the host document.
			if err = apiHost.BuildFromService(host.RunningTaskFull); err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error converting from host.Host to model.APIHost: %s", err.Error()))
			}
		}

		apiHosts = append(apiHosts, &apiHost)
	}

	return &HostsResponse{
		Hosts:              apiHosts,
		FilteredHostsCount: filteredHostsCount,
		TotalHostsCount:    totalHostsCount,
	}, nil
}

func (r *queryResolver) Host(ctx context.Context, hostID string) (*restModel.APIHost, error) {
	host, err := host.GetHostByIdOrTagWithTask(hostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error Fetching host: %s", err.Error()))
	}
	if host == nil {
		return nil, errors.Errorf("unable to find host %s", hostID)
	}

	apiHost := &restModel.APIHost{}
	err = apiHost.BuildFromService(host)
	if err != nil || apiHost == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error converting from host.Host to model.APIHost: %s", err.Error()))
	}

	if host.RunningTask != "" {
		// Add the task information to the host document.
		if err = apiHost.BuildFromService(host.RunningTaskFull); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error converting from host.Host to model.APIHost: %s", err.Error()))
		}
	}

	return apiHost, nil
}

func (r *queryResolver) MyVolumes(ctx context.Context) ([]*restModel.APIVolume, error) {
	volumes, err := GetMyVolumes(MustHaveUser(ctx))
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	volumePointers := make([]*restModel.APIVolume, 0, len(volumes))
	for i, _ := range volumes {
		volumePointers = append(volumePointers, &volumes[i])
	}
	return volumePointers, nil
}

func (r *queryResolver) MyHosts(ctx context.Context) ([]*restModel.APIHost, error) {
	usr := MustHaveUser(ctx)
	hosts, err := host.Find(host.ByUserWithRunningStatus(usr.Username()))
	if err != nil {
		return nil, InternalServerError.Send(ctx,
			fmt.Sprintf("Error finding running hosts for user %s : %s", usr.Username(), err))
	}
	var apiHosts []*restModel.APIHost

	for _, host := range hosts {
		apiHost := restModel.APIHost{}
		err = apiHost.BuildFromService(host)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APIHost from service: %s", err.Error()))
		}
		apiHosts = append(apiHosts, &apiHost)
	}
	return apiHosts, nil
}

func (r *queryResolver) ProjectSettings(ctx context.Context, identifier string) (*restModel.APIProjectSettings, error) {
	res := &restModel.APIProjectSettings{}
	projectRef, err := model.FindBranchProjectRef(identifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error looking in project collection: %s", err.Error()))
	}
	if projectRef == nil {
		// If the project ref doesn't exist for the identifier, we may be looking for a repo, so check that collection.
		repoRef, err := model.FindOneRepoRef(identifier)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("error looking in repo collection: %s", err.Error()))
		}
		if repoRef != nil {
			projectRef = &repoRef.ProjectRef
		}
	}
	if projectRef == nil {
		return nil, ResourceNotFound.Send(ctx, "project/repo doesn't exist")
	}
	apiProjectRef := restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(projectRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error building APIProjectRef from service: %s", err.Error()))
	}
	res.ProjectRef = apiProjectRef
	return res, nil
}

func (r *mutationResolver) CreateProject(ctx context.Context, project restModel.APIProjectRef) (*restModel.APIProjectRef, error) {
	projectRef, err := model.FindBranchProjectRef(*project.Identifier)
	if err != nil {
		// if the project is not found, the err will be nil based on how FindBranchProjectRef is set up
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error looking in project collection: %s", err.Error()))
	}
	if projectRef != nil {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("cannot create project with identifier '%s', identifier already in use", *project.Identifier))
	}

	i, err := project.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("API error converting from model.APIProjectRef to model.ProjectRef: %s ", err.Error()))
	}
	dbProjectRef, ok := i.(*model.ProjectRef)
	if !ok {
		return nil, InternalServerError.Send(ctx, errors.Wrapf(err, "Unexpected type %T for model.ProjectRef", i).Error())
	}

	u := gimlet.GetUser(ctx).(*user.DBUser)
	if err = r.sc.CreateProject(dbProjectRef, u); err != nil {
		return nil, InternalServerError.Send(ctx, errors.Wrapf(err, "Database error for insert() project with project name '%s'", *project.Identifier).Error())
	}

	projectRef, err = model.FindBranchProjectRef(*project.Identifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error looking in project collection: %s", err.Error()))
	}
	if projectRef == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding project: %s", err.Error()))
	}
	apiProjectRef := restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(projectRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error building APIProjectRef from service: %s", err.Error()))
	}

	return &apiProjectRef, nil
}

func (r *mutationResolver) AttachVolumeToHost(ctx context.Context, volumeAndHost VolumeHost) (bool, error) {
	success, _, gqlErr, err := AttachVolume(ctx, volumeAndHost.VolumeID, volumeAndHost.HostID)
	if err != nil {
		return false, gqlErr.Send(ctx, err.Error())
	}
	return success, nil
}

func (r *mutationResolver) DetachVolumeFromHost(ctx context.Context, volumeID string) (bool, error) {
	success, _, gqlErr, err := DetachVolume(ctx, volumeID)
	if err != nil {
		return false, gqlErr.Send(ctx, err.Error())
	}
	return success, nil
}

type patchResolver struct{ *Resolver }

func (r *patchResolver) CommitQueuePosition(ctx context.Context, apiPatch *restModel.APIPatch) (*int, error) {
	var commitQueuePosition *int
	if *apiPatch.Alias == evergreen.CommitQueueAlias {
		cq, err := commitqueue.FindOneId(*apiPatch.ProjectId)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting commit queue position for patch %s: %s", *apiPatch.Id, err.Error()))
		}
		if cq != nil {
			position := cq.FindItem(*apiPatch.Id)
			commitQueuePosition = &position
		}
	}
	return commitQueuePosition, nil
}

func (r *patchResolver) ProjectIdentifier(ctx context.Context, apiPatch *restModel.APIPatch) (*string, error) {
	identifier, err := model.GetIdentifierForProject(*apiPatch.ProjectId)
	if err != nil {
		return apiPatch.ProjectId, nil
	}
	return utility.ToStringPtr(identifier), nil
}

func (r *patchResolver) AuthorDisplayName(ctx context.Context, obj *restModel.APIPatch) (string, error) {
	usr, err := user.FindOneById(*obj.Author)
	if err != nil {
		return "", ResourceNotFound.Send(ctx, fmt.Sprintf("Error getting user from user ID: %s", err.Error()))
	}
	if usr == nil {
		return "", ResourceNotFound.Send(ctx, fmt.Sprint("Could not find user from user ID"))
	}
	return usr.DisplayName(), nil
}

func (r *patchResolver) TaskStatuses(ctx context.Context, obj *restModel.APIPatch) ([]string, error) {
	defaultSort := []task.TasksSortOrder{
		{Key: task.DisplayNameKey, Order: 1},
	}
	opts := data.TaskFilterOptions{
		Sorts:            defaultSort,
		IncludeBaseTasks: false,
	}
	tasks, _, err := r.sc.FindTasksByVersion(*obj.Id, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting version tasks: %s", err.Error()))
	}
	return getAllTaskStatuses(tasks), nil
}

func (r *patchResolver) BaseTaskStatuses(ctx context.Context, obj *restModel.APIPatch) ([]string, error) {
	baseTasks, err := getVersionBaseTasks(r.sc, *obj.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting version base tasks: %s", err.Error()))
	}
	return getAllTaskStatuses(baseTasks), nil
}

func (r *patchResolver) Builds(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APIBuild, error) {
	builds, err := build.FindBuildsByVersions([]string{*obj.Version})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding build by version %s: %s", *obj.Version, err.Error()))
	}
	var apiBuilds []*restModel.APIBuild
	for _, build := range builds {
		apiBuild := restModel.APIBuild{}
		err = apiBuild.BuildFromService(build)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APIBuild from service: %s", err.Error()))
		}
		apiBuilds = append(apiBuilds, &apiBuild)
	}
	return apiBuilds, nil
}

func (r *patchResolver) Duration(ctx context.Context, obj *restModel.APIPatch) (*PatchDuration, error) {
	tasks, err := task.FindAllFirstExecution(task.ByVersion(*obj.Id).WithFields(task.TimeTakenKey, task.StartTimeKey, task.FinishTimeKey, task.DisplayOnlyKey, task.ExecutionKey))
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	if tasks == nil {
		return nil, ResourceNotFound.Send(ctx, "Could not find any tasks for patch")
	}
	timeTaken, makespan := task.GetTimeSpent(tasks)

	// return nil if rounded timeTaken/makespan == 0s
	t := timeTaken.Round(time.Second).String()
	var tPointer *string
	if t != "0s" {
		tFormated := formatDuration(t)
		tPointer = &tFormated
	}
	m := makespan.Round(time.Second).String()
	var mPointer *string
	if m != "0s" {
		mFormated := formatDuration(m)
		mPointer = &mFormated
	}

	return &PatchDuration{
		Makespan:  mPointer,
		TimeTaken: tPointer,
	}, nil
}

func (r *patchResolver) Time(ctx context.Context, obj *restModel.APIPatch) (*PatchTime, error) {
	usr := MustHaveUser(ctx)

	started, err := GetFormattedDate(obj.StartTime, usr.Settings.Timezone)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	finished, err := GetFormattedDate(obj.FinishTime, usr.Settings.Timezone)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	submittedAt, err := GetFormattedDate(obj.CreateTime, usr.Settings.Timezone)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}

	return &PatchTime{
		Started:     started,
		Finished:    finished,
		SubmittedAt: *submittedAt,
	}, nil
}

func (r *patchResolver) TaskCount(ctx context.Context, obj *restModel.APIPatch) (*int, error) {
	taskCount, err := task.Count(task.ByVersion(*obj.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting task count for patch %s: %s", *obj.Id, err.Error()))
	}
	return &taskCount, nil
}

func (r *patchResolver) BaseVersionID(ctx context.Context, obj *restModel.APIPatch) (*string, error) {
	baseVersion, err := model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(*obj.ProjectId, *obj.Githash).Project(bson.M{model.VersionIdentifierKey: 1}))
	if baseVersion == nil || err != nil {
		return nil, nil
	}
	return &baseVersion.Id, nil
}

func (r *patchResolver) Project(ctx context.Context, apiPatch *restModel.APIPatch) (*PatchProject, error) {
	patchProject, err := GetPatchProjectVariantsAndTasksForUI(ctx, apiPatch)
	if err != nil {
		return nil, err
	}
	return patchProject, nil
}

func (r *patchResolver) ID(ctx context.Context, obj *restModel.APIPatch) (string, error) {
	return *obj.Id, nil
}

func (r *patchResolver) PatchTriggerAliases(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APIPatchTriggerDefinition, error) {
	projectRef, err := r.sc.FindProjectById(*obj.ProjectId, true)
	if err != nil || projectRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project: %s : %s", *obj.ProjectId, err))
	}

	if len(projectRef.PatchTriggerAliases) == 0 {
		return nil, nil
	}

	projectCache := map[string]*model.Project{}
	aliases := []*restModel.APIPatchTriggerDefinition{}
	for _, alias := range projectRef.PatchTriggerAliases {
		project, projectCached := projectCache[alias.ChildProject]
		if !projectCached {
			_, project, err = model.FindLatestVersionWithValidProject(alias.ChildProject)
			if err != nil {
				return nil, InternalServerError.Send(ctx, errors.Wrapf(err, "Problem getting last known project for '%s'", alias.ChildProject).Error())
			}
			projectCache[alias.ChildProject] = project
		}

		matchingTasks, err := project.VariantTasksForSelectors([]patch.PatchTriggerDefinition{alias}, *obj.Requester)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Problem matching tasks to alias definitions: %v", err.Error()))
		}

		variantsTasks := []restModel.VariantTask{}
		for _, vt := range matchingTasks {
			variantsTasks = append(variantsTasks, restModel.VariantTask{
				Name:  utility.ToStringPtr(vt.Variant),
				Tasks: utility.ToStringPtrSlice(vt.Tasks),
			})
		}

		identifier, err := model.GetIdentifierForProject(alias.ChildProject)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Problem getting child project identifier: %v", err.Error()))
		}

		aliases = append(aliases, &restModel.APIPatchTriggerDefinition{
			Alias:                  utility.ToStringPtr(alias.Alias),
			ChildProject:           utility.ToStringPtr(alias.ChildProject),
			ChildProjectId:         utility.ToStringPtr(alias.ChildProject),
			ChildProjectIdentifier: utility.ToStringPtr(identifier),
			VariantsTasks:          variantsTasks,
		})
	}

	return aliases, nil
}

func (r *queryResolver) Patch(ctx context.Context, id string) (*restModel.APIPatch, error) {
	patch, err := r.sc.FindPatchById(id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}

	if evergreen.IsFinishedPatchStatus(*patch.Status) {
		failedAndAbortedStatuses := append(evergreen.TaskFailureStatuses, evergreen.TaskAborted)
		opts := data.TaskFilterOptions{
			Statuses:         failedAndAbortedStatuses,
			FieldsToProject:  []string{task.DisplayStatusKey},
			IncludeBaseTasks: false,
		}
		tasks, _, err := r.sc.FindTasksByVersion(id, opts)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Could not fetch tasks for patch: %s ", err.Error()))
		}

		if len(patch.ChildPatches) > 0 {
			for _, cp := range patch.ChildPatches {
				// add the child patch tasks to tasks so that we can consider their status
				childPatchTasks, _, err := r.sc.FindTasksByVersion(*cp.Id, opts)
				if err != nil {
					return nil, InternalServerError.Send(ctx, fmt.Sprintf("Could not fetch tasks for child patch: %s ", err.Error()))
				}
				tasks = append(tasks, childPatchTasks...)
			}
		}
		statuses := getAllTaskStatuses(tasks)

		// If theres an aborted task we should set the patch status to aborted if there are no other failures
		if utility.StringSliceContains(statuses, evergreen.TaskAborted) {
			if len(utility.StringSliceIntersection(statuses, evergreen.TaskFailureStatuses)) == 0 {
				patch.Status = utility.ToStringPtr(evergreen.PatchAborted)
			}
		}
	}

	return patch, nil
}

func (r *queryResolver) Version(ctx context.Context, id string) (*restModel.APIVersion, error) {
	v, err := r.sc.FindVersionById(id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while finding version with id: `%s`: %s", id, err.Error()))
	}
	apiVersion := restModel.APIVersion{}
	if err = apiVersion.BuildFromService(v); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APIVersion from service for `%s`: %s", id, err.Error()))
	}
	return &apiVersion, nil
}

func (r *queryResolver) Project(ctx context.Context, id string) (*restModel.APIProjectRef, error) {
	project, err := r.sc.FindProjectById(id, true)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding project by id %s: %s", id, err.Error()))
	}
	apiProjectRef := restModel.APIProjectRef{}
	err = apiProjectRef.BuildFromService(project)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error building APIProject from service: %s", err.Error()))
	}
	return &apiProjectRef, nil
}

func (r *projectResolver) Patches(ctx context.Context, obj *restModel.APIProjectRef, patchesInput PatchesInput) (*Patches, error) {
	patches, count, err := r.sc.FindPatchesByProjectPatchNameStatusesCommitQueue(*obj.Id, patchesInput.PatchName, patchesInput.Statuses, patchesInput.IncludeCommitQueue, patchesInput.Page, patchesInput.Limit)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	patchPointers := []*restModel.APIPatch{}
	for i := range patches {
		patchPointers = append(patchPointers, &patches[i])
	}

	return &Patches{Patches: patchPointers, FilteredPatchCount: *count}, nil
}

func (r *queryResolver) UserSettings(ctx context.Context) (*restModel.APIUserSettings, error) {
	usr := MustHaveUser(ctx)
	userSettings := restModel.APIUserSettings{}
	err := userSettings.BuildFromService(usr.Settings)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return &userSettings, nil
}

func (r *queryResolver) UserPatches(ctx context.Context, limit *int, page *int, patchName *string, statuses []string, userID *string, includeCommitQueue *bool) (*UserPatches, error) {
	usr := MustHaveUser(ctx)
	userIdParam := usr.Username()
	if userID != nil {
		userIdParam = *userID
	}
	patches, count, err := r.sc.FindPatchesByUserPatchNameStatusesCommitQueue(userIdParam, *patchName, statuses, *includeCommitQueue, *page, *limit)
	patchPointers := []*restModel.APIPatch{}
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	for i := range patches {
		patchPointers = append(patchPointers, &patches[i])
	}
	userPatches := UserPatches{
		Patches:            patchPointers,
		FilteredPatchCount: *count,
	}
	return &userPatches, nil
}

func (r *queryResolver) Task(ctx context.Context, taskID string, execution *int) (*restModel.APITask, error) {
	dbTask, err := task.FindOneIdAndExecutionWithDisplayStatus(taskID, execution)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if dbTask == nil {
		return nil, errors.Errorf("unable to find task %s", taskID)
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *dbTask)
	if err != nil {
		return nil, InternalServerError.Send(ctx, "error converting task")
	}

	return apiTask, err
}

func (r *queryResolver) TaskAllExecutions(ctx context.Context, taskID string) ([]*restModel.APITask, error) {
	latestTask, err := task.FindOneId(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if latestTask == nil {
		return nil, errors.Errorf("unable to find task %s", taskID)
	}
	allTasks := []*restModel.APITask{}
	for i := 0; i < latestTask.Execution; i++ {
		var dbTask *task.Task
		dbTask, err = task.FindByIdExecution(taskID, &i)
		if err != nil {
			return nil, ResourceNotFound.Send(ctx, err.Error())
		}
		if dbTask == nil {
			return nil, errors.Errorf("unable to find task %s", taskID)
		}
		var apiTask *restModel.APITask
		apiTask, err = GetAPITaskFromTask(ctx, r.sc, *dbTask)
		if err != nil {
			return nil, InternalServerError.Send(ctx, "error converting task")
		}
		allTasks = append(allTasks, apiTask)
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *latestTask)
	if err != nil {
		return nil, InternalServerError.Send(ctx, "error converting task")
	}
	allTasks = append(allTasks, apiTask)
	return allTasks, nil
}

func (r *queryResolver) Projects(ctx context.Context) ([]*GroupedProjects, error) {
	allProjs, err := model.FindAllMergedTrackedProjectRefs()
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}

	groupsMap := make(map[string][]*restModel.APIProjectRef)

	for _, p := range allProjs {
		groupName := strings.Join([]string{p.Owner, p.Repo}, "/")
		apiProjectRef := restModel.APIProjectRef{}
		if err = apiProjectRef.BuildFromService(p); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("error building APIProjectRef from service: %s", err.Error()))
		}

		if projs, ok := groupsMap[groupName]; ok {
			groupsMap[groupName] = append(projs, &apiProjectRef)
		} else {
			groupsMap[groupName] = []*restModel.APIProjectRef{&apiProjectRef}
		}
	}

	groupsArr := []*GroupedProjects{}

	for groupName, groupedProjects := range groupsMap {
		gp := GroupedProjects{
			Name:     groupName,
			Projects: groupedProjects,
		}
		groupsArr = append(groupsArr, &gp)
	}

	sort.SliceStable(groupsArr, func(i, j int) bool {
		return groupsArr[i].Name < groupsArr[j].Name
	})

	return groupsArr, nil
}

func (r *queryResolver) PatchTasks(ctx context.Context, patchID string, sorts []*SortOrder, page *int, limit *int, statuses []string, baseStatuses []string, variant *string, taskName *string) (*PatchTasks, error) {
	pageParam := 0
	if page != nil {
		pageParam = *page
	}
	limitParam := 0
	if limit != nil {
		limitParam = *limit
	}
	variantParam := ""
	if variant != nil {
		variantParam = *variant
	}
	taskNameParam := ""
	if taskName != nil {
		taskNameParam = *taskName
	}
	var taskSorts []task.TasksSortOrder
	if len(sorts) > 0 {
		taskSorts = []task.TasksSortOrder{}
		for _, singleSort := range sorts {
			key := ""
			switch singleSort.Key {
			// the keys here should be the keys for the column headers of the tasks table
			case TaskSortCategoryName:
				key = task.DisplayNameKey
			case TaskSortCategoryStatus:
				key = task.DisplayStatusKey
			case TaskSortCategoryBaseStatus:
				key = task.BaseTaskStatusKey
			case TaskSortCategoryVariant:
				key = task.BuildVariantKey
			default:
				return nil, InputValidationError.Send(ctx, fmt.Sprintf("invalid sort key: %s", singleSort.Key))
			}
			order := 1
			if singleSort.Direction == SortDirectionDesc {
				order = -1
			}
			taskSorts = append(taskSorts, task.TasksSortOrder{Key: key, Order: order})
		}
	}
	opts := data.TaskFilterOptions{
		Statuses:         statuses,
		BaseStatuses:     baseStatuses,
		Variants:         []string{variantParam},
		TaskNames:        []string{taskNameParam},
		Page:             pageParam,
		Limit:            limitParam,
		Sorts:            taskSorts,
		IncludeBaseTasks: true,
	}
	tasks, count, err := r.sc.FindTasksByVersion(patchID, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting patch tasks for %s: %s", patchID, err.Error()))
	}

	var apiTasks []*restModel.APITask
	for _, t := range tasks {
		apiTask := restModel.APITask{}
		err := apiTask.BuildFromService(&t)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error converting task item db model to api model: %v", err.Error()))
		}
		apiTasks = append(apiTasks, &apiTask)
	}
	patchTasks := PatchTasks{
		Count: count,
		Tasks: apiTasks,
	}
	return &patchTasks, nil
}

func (r *queryResolver) TaskTests(ctx context.Context, taskID string, execution *int, sortCategory *TestSortCategory, sortDirection *SortDirection, page *int, limit *int, testName *string, statuses []string, groupID *string) (*TaskTestResult, error) {
	dbTask, err := task.FindByIdExecution(taskID, execution)
	if dbTask == nil || err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("finding task with id %s", taskID))
	}
	baseTask, err := dbTask.FindTaskOnBaseCommit()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding base task for task %s: %s", taskID, err))
	}

	var sortBy, cedarSortBy string
	if sortCategory != nil {
		switch *sortCategory {
		case TestSortCategoryStatus:
			cedarSortBy = apimodels.CedarTestResultsSortByStatus
			sortBy = testresult.StatusKey
		case TestSortCategoryDuration:
			cedarSortBy = apimodels.CedarTestResultsSortByDuration
			sortBy = "duration"
		case TestSortCategoryTestName:
			cedarSortBy = apimodels.CedarTestResultsSortByTestName
			sortBy = testresult.TestFileKey
		case TestSortCategoryStartTime:
			cedarSortBy = apimodels.CedarTestResultsSortByStart
			sortBy = testresult.StartTimeKey
		case TestSortCategoryBaseStatus:
			cedarSortBy = apimodels.CedarTestResultsSortByBaseStatus
			sortBy = "base_status"
		}
	}

	if dbTask.HasCedarResults {
		opts := apimodels.GetCedarTestResultsOptions{
			BaseURL:      evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:       taskID,
			Execution:    utility.ToIntPtr(dbTask.Execution),
			DisplayTask:  dbTask.DisplayOnly,
			TestName:     utility.FromStringPtr(testName),
			Statuses:     statuses,
			GroupID:      utility.FromStringPtr(groupID),
			SortBy:       cedarSortBy,
			SortOrderDSC: sortDirection != nil && *sortDirection == SortDirectionDesc,
			Limit:        utility.FromIntPtr(limit),
			Page:         utility.FromIntPtr(page),
		}
		if baseTask != nil && baseTask.HasCedarResults {
			opts.BaseTaskID = baseTask.Id
		}
		cedarTestResults, err := apimodels.GetCedarTestResults(ctx, opts)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding test results for task %s: %s", taskID, err))
		}

		apiTestResults := make([]*restModel.APITest, len(cedarTestResults.Results))
		for i, t := range cedarTestResults.Results {
			apiTest := &restModel.APITest{}
			if err = apiTest.BuildFromService(t.TaskID); err != nil {
				return nil, InternalServerError.Send(ctx, err.Error())
			}
			if err = apiTest.BuildFromService(&t); err != nil {
				return nil, InternalServerError.Send(ctx, err.Error())
			}

			apiTestResults[i] = apiTest
		}

		return &TaskTestResult{
			TestResults:       apiTestResults,
			TotalTestCount:    cedarTestResults.Stats.TotalCount,
			FilteredTestCount: utility.FromIntPtr(cedarTestResults.Stats.FilteredCount),
		}, nil
	}

	baseTestStatusMap := map[string]string{}
	if baseTask != nil {
		baseTestResults, _ := r.sc.FindTestsByTaskId(data.FindTestsByTaskIdOpts{TaskID: baseTask.Id, Execution: baseTask.Execution})
		for _, t := range baseTestResults {
			baseTestStatusMap[t.TestFile] = t.Status
		}
	}
	sortDir := 1
	if sortDirection != nil && *sortDirection == SortDirectionDesc {
		sortDir = -1
	}
	filteredTestResults, err := r.sc.FindTestsByTaskId(data.FindTestsByTaskIdOpts{
		TaskID:    taskID,
		Execution: dbTask.Execution,
		TestName:  utility.FromStringPtr(testName),
		Statuses:  statuses,
		SortBy:    sortBy,
		SortDir:   sortDir,
		GroupID:   utility.FromStringPtr(groupID),
		Limit:     utility.FromIntPtr(limit),
		Page:      utility.FromIntPtr(page),
	})
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}

	apiTestResults := make([]*restModel.APITest, len(filteredTestResults))
	for i, t := range filteredTestResults {
		apiTest := &restModel.APITest{}
		if err = apiTest.BuildFromService(t.TaskID); err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		if err = apiTest.BuildFromService(&t); err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		apiTest.BaseStatus = utility.ToStringPtr(baseTestStatusMap[utility.FromStringPtr(apiTest.TestFile)])

		apiTestResults[i] = apiTest
	}
	totalTestCount, err := r.sc.GetTestCountByTaskIdAndFilters(taskID, "", []string{}, dbTask.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting total test count: %s", err))
	}
	filteredTestCount, err := r.sc.GetTestCountByTaskIdAndFilters(taskID, utility.FromStringPtr(testName), statuses, dbTask.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting filtered test count: %s", err))
	}

	return &TaskTestResult{
		TestResults:       apiTestResults,
		TotalTestCount:    totalTestCount,
		FilteredTestCount: filteredTestCount,
	}, nil
}

func (r *queryResolver) TaskFiles(ctx context.Context, taskID string, execution *int) (*TaskFiles, error) {
	emptyTaskFiles := TaskFiles{
		FileCount:    0,
		GroupedFiles: []*GroupedFiles{},
	}
	t, err := task.FindByIdExecution(taskID, execution)
	if t == nil {
		return &emptyTaskFiles, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	if err != nil {
		return &emptyTaskFiles, ResourceNotFound.Send(ctx, err.Error())
	}
	groupedFilesList := []*GroupedFiles{}
	fileCount := 0
	if t.DisplayOnly {
		execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
		if err != nil {
			return &emptyTaskFiles, ResourceNotFound.Send(ctx, err.Error())
		}
		for _, execTask := range execTasks {
			groupedFiles, err := GetGroupedFiles(ctx, execTask.DisplayName, execTask.Id, t.Execution)
			if err != nil {
				return &emptyTaskFiles, err
			}
			fileCount += len(groupedFiles.Files)
			groupedFilesList = append(groupedFilesList, groupedFiles)
		}
	} else {
		groupedFiles, err := GetGroupedFiles(ctx, t.DisplayName, taskID, t.Execution)
		if err != nil {
			return &emptyTaskFiles, err
		}
		fileCount += len(groupedFiles.Files)
		groupedFilesList = append(groupedFilesList, groupedFiles)
	}
	taskFiles := TaskFiles{
		FileCount:    fileCount,
		GroupedFiles: groupedFilesList,
	}
	return &taskFiles, nil
}

func (r *queryResolver) TaskLogs(ctx context.Context, taskID string, execution *int) (*TaskLogs, error) {
	t, err := task.FindByIdExecution(taskID, execution)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	// need project to get default logger
	p, err := r.sc.FindProjectById(t.Project, true)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding project '%s': %s", t.Project, err.Error()))
	}
	if p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("could not find project '%s'", t.Project))
	}
	defaultLogger := p.DefaultLogger
	if defaultLogger == "" {
		defaultLogger = evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger
	}

	// Let the individual TaskLogs resolvers handle fetching logs for the task
	// We can avoid the overhead of fetching task logs that we will not view
	// and we can avoid handling errors that we will not see
	return &TaskLogs{TaskID: taskID, Execution: t.Execution, DefaultLogger: defaultLogger}, nil
}

func (r *taskLogsResolver) SystemLogs(ctx context.Context, obj *TaskLogs) ([]*apimodels.LogMessage, error) {
	const logMessageCount = 100

	var systemLogs []apimodels.LogMessage

	// get logs from cedar
	if obj.DefaultLogger == model.BuildloggerLogSender {
		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:        obj.TaskID,
			Execution:     utility.ToIntPtr(obj.Execution),
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.TaskLogPrefix,
		}

		// system logs
		opts.LogType = apimodels.SystemLogPrefix
		systemLogReader, err := apimodels.GetBuildloggerLogs(ctx, opts)
		if err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		systemLogs = apimodels.ReadBuildloggerToSlice(ctx, obj.TaskID, systemLogReader)
	} else {
		var err error

		systemLogs, err = model.FindMostRecentLogMessages(obj.TaskID, obj.Execution, logMessageCount, []string{},
			[]string{apimodels.SystemLogPrefix})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding system logs for task %s: %s", obj.TaskID, err.Error()))
		}
	}
	systemLogPointers := []*apimodels.LogMessage{}
	for i := range systemLogs {
		systemLogPointers = append(systemLogPointers, &systemLogs[i])
	}

	return systemLogPointers, nil
}
func (r *taskLogsResolver) EventLogs(ctx context.Context, obj *TaskLogs) ([]*restModel.TaskAPIEventLogEntry, error) {
	const logMessageCount = 100
	var loggedEvents []event.EventLogEntry
	// loggedEvents is ordered ts descending
	loggedEvents, err := event.Find(event.AllLogCollection, event.MostRecentTaskEvents(obj.TaskID, logMessageCount))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to find EventLogs for task %s: %s", obj.TaskID, err.Error()))
	}

	// remove all scheduled events except the youngest and push to filteredEvents
	filteredEvents := []event.EventLogEntry{}
	foundScheduled := false
	for i := 0; i < len(loggedEvents); i++ {
		if !foundScheduled || loggedEvents[i].EventType != event.TaskScheduled {
			filteredEvents = append(filteredEvents, loggedEvents[i])
		}
		if loggedEvents[i].EventType == event.TaskScheduled {
			foundScheduled = true
		}
	}

	// reverse order so it is ascending
	for i := len(filteredEvents)/2 - 1; i >= 0; i-- {
		opp := len(filteredEvents) - 1 - i
		filteredEvents[i], filteredEvents[opp] = filteredEvents[opp], filteredEvents[i]
	}

	// populate eventlogs pointer arrays
	apiEventLogPointers := []*restModel.TaskAPIEventLogEntry{}
	for _, e := range filteredEvents {
		apiEventLog := restModel.TaskAPIEventLogEntry{}
		err = apiEventLog.BuildFromService(&e)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to build APIEventLogEntry from EventLog: %s", err.Error()))
		}
		apiEventLogPointers = append(apiEventLogPointers, &apiEventLog)
	}
	return apiEventLogPointers, nil
}
func (r *taskLogsResolver) AgentLogs(ctx context.Context, obj *TaskLogs) ([]*apimodels.LogMessage, error) {
	const logMessageCount = 100

	var agentLogs []apimodels.LogMessage
	// get logs from cedar
	if obj.DefaultLogger == model.BuildloggerLogSender {
		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:        obj.TaskID,
			Execution:     utility.ToIntPtr(obj.Execution),
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.AgentLogPrefix,
		}
		// agent logs
		agentLogReader, err := apimodels.GetBuildloggerLogs(ctx, opts)
		if err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		agentLogs = apimodels.ReadBuildloggerToSlice(ctx, obj.TaskID, agentLogReader)
	} else {
		var err error
		// agent logs
		agentLogs, err = model.FindMostRecentLogMessages(obj.TaskID, obj.Execution, logMessageCount, []string{},
			[]string{apimodels.AgentLogPrefix})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding agent logs for task %s: %s", obj.TaskID, err.Error()))
		}
	}

	agentLogPointers := []*apimodels.LogMessage{}

	for i := range agentLogs {
		agentLogPointers = append(agentLogPointers, &agentLogs[i])
	}
	return agentLogPointers, nil
}
func (r *taskLogsResolver) TaskLogs(ctx context.Context, obj *TaskLogs) ([]*apimodels.LogMessage, error) {
	const logMessageCount = 100

	var taskLogs []apimodels.LogMessage

	// get logs from cedar
	if obj.DefaultLogger == model.BuildloggerLogSender {

		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:        obj.TaskID,
			Execution:     utility.ToIntPtr(obj.Execution),
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.TaskLogPrefix,
		}
		// task logs
		taskLogReader, err := apimodels.GetBuildloggerLogs(ctx, opts)

		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Encountered an error while fetching build logger logs: %s", err.Error()))
		}

		taskLogs = apimodels.ReadBuildloggerToSlice(ctx, obj.TaskID, taskLogReader)

	} else {
		var err error

		// task logs
		taskLogs, err = model.FindMostRecentLogMessages(obj.TaskID, obj.Execution, logMessageCount, []string{},
			[]string{apimodels.TaskLogPrefix})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding task logs for task %s: %s", obj.TaskID, err.Error()))
		}
	}

	taskLogPointers := []*apimodels.LogMessage{}
	for i := range taskLogs {
		taskLogPointers = append(taskLogPointers, &taskLogs[i])
	}

	return taskLogPointers, nil
}

func (r *queryResolver) PatchBuildVariants(ctx context.Context, patchID string) ([]*GroupedBuildVariant, error) {
	patch, err := r.sc.FindPatchById(patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding patch `%s`: %s", patchID, err))
	}
	groupedBuildVariants, err := generateBuildVariants(r.sc, *patch.Id, []string{}, []string{}, []string{})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error generating grouped build variants: %s", err))
	}
	return groupedBuildVariants, nil
}

func (r *queryResolver) CommitQueue(ctx context.Context, id string) (*restModel.APICommitQueue, error) {
	commitQueue, err := r.sc.FindCommitQueueForProject(id)
	if err != nil {
		if errors.Cause(err) == err {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding commit queue for %s: %s", id, err.Error()))
		}
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding commit queue for %s: %s", id, err.Error()))
	}
	project, err := r.sc.FindProjectById(id, true)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding project %s: %s", id, err.Error()))
	}
	if project.CommitQueue.Message != "" {
		commitQueue.Message = &project.CommitQueue.Message
	}
	commitQueue.Owner = &project.Owner
	commitQueue.Repo = &project.Repo

	for i, item := range commitQueue.Queue {
		patchId := ""
		if utility.FromStringPtr(item.Version) != "" {
			patchId = utility.FromStringPtr(item.Version)
		} else if utility.FromStringPtr(item.PatchId) != "" {
			patchId = utility.FromStringPtr(item.PatchId)
		}

		if patchId != "" {
			p, err := r.sc.FindPatchById(patchId)
			if err != nil {
				return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding patch: %s", err.Error()))
			}
			commitQueue.Queue[i].Patch = p
		}
	}

	return commitQueue, nil
}

func (r *queryResolver) UserConfig(ctx context.Context) (*UserConfig, error) {
	usr := MustHaveUser(ctx)
	settings := evergreen.GetEnvironment().Settings()
	config := &UserConfig{
		User:          usr.Username(),
		APIKey:        usr.GetAPIKey(),
		UIServerHost:  settings.Ui.Url,
		APIServerHost: settings.ApiUrl + "/api",
	}

	return config, nil
}

func (r *queryResolver) ClientConfig(ctx context.Context) (*restModel.APIClientConfig, error) {
	envClientConfig := evergreen.GetEnvironment().ClientConfig()
	clientConfig := restModel.APIClientConfig{}
	err := clientConfig.BuildFromService(*envClientConfig)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APIClientConfig from service: %s", err.Error()))
	}

	return &clientConfig, nil
}

func (r *queryResolver) AwsRegions(ctx context.Context) ([]string, error) {
	return evergreen.GetEnvironment().Settings().Providers.AWS.AllowedRegions, nil
}

func (r *queryResolver) SubnetAvailabilityZones(ctx context.Context) ([]string, error) {
	zones := []string{}
	for _, subnet := range evergreen.GetEnvironment().Settings().Providers.AWS.Subnets {
		zones = append(zones, subnet.AZ)
	}
	return zones, nil
}

func (r *queryResolver) SpruceConfig(ctx context.Context) (*restModel.APIAdminSettings, error) {
	config, err := evergreen.GetConfig()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error Fetching evergreen settings: %s", err.Error()))
	}

	spruceConfig := restModel.APIAdminSettings{}
	err = spruceConfig.BuildFromService(config)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building api admin settings from service: %s", err.Error()))
	}
	return &spruceConfig, nil
}

func (r *queryResolver) HostEvents(ctx context.Context, hostID string, hostTag *string, limit *int, page *int) (*HostEvents, error) {
	h, err := host.FindOneByIdOrTag(hostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding host %s: %s", hostID, err.Error()))
	}
	events, count, err := event.FindPaginated(h.Id, h.Tag, event.AllLogCollection, *limit, *page)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error Fetching host events: %s", err.Error()))
	}
	// populate eventlogs pointer arrays
	apiEventLogPointers := []*restModel.HostAPIEventLogEntry{}
	for _, e := range events {
		apiEventLog := restModel.HostAPIEventLogEntry{}
		err = apiEventLog.BuildFromService(&e)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to build APIEventLogEntry from EventLog: %s", err.Error()))
		}
		apiEventLogPointers = append(apiEventLogPointers, &apiEventLog)
	}
	hostevents := HostEvents{
		EventLogEntries: apiEventLogPointers,
		Count:           count,
	}
	return &hostevents, nil
}

func (r *queryResolver) Distros(ctx context.Context, onlySpawnable bool) ([]*restModel.APIDistro, error) {
	apiDistros := []*restModel.APIDistro{}

	var distros []distro.Distro
	if onlySpawnable {
		d, err := distro.Find(distro.BySpawnAllowed())
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while fetching spawnable distros: %s", err.Error()))
		}
		distros = d
	} else {
		d, err := distro.FindAll()
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while fetching distros: %s", err.Error()))
		}
		distros = d
	}
	for _, d := range distros {
		apiDistro := restModel.APIDistro{}
		err := apiDistro.BuildFromService(d)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to build APIDistro from distro: %s", err.Error()))
		}
		apiDistros = append(apiDistros, &apiDistro)
	}
	return apiDistros, nil
}

func (r *queryResolver) DistroTaskQueue(ctx context.Context, distroID string) ([]*restModel.APITaskQueueItem, error) {
	distroQueue, err := model.LoadTaskQueue(distroID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting task queue for distro %v: %v", distroID, err.Error()))
	}
	if distroQueue == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find queue with distro ID `%s`", distroID))
	}

	taskQueue := []*restModel.APITaskQueueItem{}

	for _, taskQueueItem := range distroQueue.Queue {
		apiTaskQueueItem := restModel.APITaskQueueItem{}

		err := apiTaskQueueItem.BuildFromService(taskQueueItem)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error converting task queue item db model to api model: %v", err.Error()))
		}

		taskQueue = append(taskQueue, &apiTaskQueueItem)
	}

	return taskQueue, nil
}

func (r *queryResolver) TaskQueueDistros(ctx context.Context) ([]*TaskQueueDistro, error) {
	queues, err := model.FindAllTaskQueues()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting all task queues: %v", err.Error()))
	}

	distros := []*TaskQueueDistro{}

	for _, distro := range queues {
		tqd := TaskQueueDistro{
			ID:         distro.Distro,
			QueueCount: len(distro.Queue),
		}
		distros = append(distros, &tqd)
	}

	// sort distros by queue count in descending order
	sort.SliceStable(distros, func(i, j int) bool {
		return distros[i].QueueCount > distros[j].QueueCount
	})

	return distros, nil
}

func (r *taskQueueItemResolver) Requester(ctx context.Context, obj *restModel.APITaskQueueItem) (TaskQueueItemType, error) {
	if *obj.Requester != evergreen.RepotrackerVersionRequester {
		return TaskQueueItemTypePatch, nil
	}
	return TaskQueueItemTypeCommit, nil
}

func (r *mutationResolver) AttachProjectToRepo(ctx context.Context, projectID string) (*restModel.APIProjectRef, error) {
	usr := MustHaveUser(ctx)
	pRef, err := r.sc.FindProjectById(projectID, false)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding project %s: %s", projectID, err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find project %s", projectID))
	}
	if err = pRef.AttachToRepo(usr); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error attaching to repo: %s", err.Error()))
	}

	res := &restModel.APIProjectRef{}
	if err := res.BuildFromService(pRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error building project from service: %s", err.Error()))
	}
	return res, nil
}

func (r *mutationResolver) DetachProjectFromRepo(ctx context.Context, projectID string) (*restModel.APIProjectRef, error) {
	usr := MustHaveUser(ctx)
	pRef, err := r.sc.FindProjectById(projectID, false)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding project %s: %s", projectID, err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find project %s", projectID))
	}
	if err = pRef.DetachFromRepo(usr); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error detaching from repo: %s", err.Error()))
	}

	res := &restModel.APIProjectRef{}
	if err := res.BuildFromService(pRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error building project from service: %s", err.Error()))
	}
	return res, nil
}

func (r *mutationResolver) SetTaskPriority(ctx context.Context, taskID string, priority int) (*restModel.APITask, error) {
	t, err := r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
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
			return nil, Forbidden.Send(ctx, fmt.Sprintf("Insufficient access to set priority %v, can only set priority less than or equal to %v", priority, evergreen.MaxTaskPriority))
		}
	}
	if err = model.SetTaskPriority(*t, int64(priority), authUser.Username()); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error setting task priority %v: %v", taskID, err.Error()))
	}

	t, err = r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *t)
	return apiTask, err
}

func (r *mutationResolver) OverrideTaskDependencies(ctx context.Context, taskID string) (*restModel.APITask, error) {
	currentUser := MustHaveUser(ctx)
	t, err := task.FindByIdExecution(taskID, nil)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	if err = t.SetOverrideDependencies(currentUser.Username()); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error overriding dependencies for task %s: %s", taskID, err.Error()))
	}
	t.DisplayStatus = t.GetDisplayStatus()
	return GetAPITaskFromTask(ctx, r.sc, *t)
}

func (r *mutationResolver) SchedulePatch(ctx context.Context, patchID string, configure PatchConfigure) (*restModel.APIPatch, error) {
	patchUpdateReq := PatchUpdate{}
	patchUpdateReq.BuildFromGqlInput(configure)
	version, err := r.sc.FindVersionById(patchID)
	if err != nil {
		// FindVersionById does not distinguish between nil version err and db err; therefore must check that err
		// does not contain nil version err values before sending InternalServerError
		if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error occurred fetching patch `%s`: %s", patchID, err.Error()))
		}
	}
	err, _, _, versionID := SchedulePatch(ctx, patchID, version, patchUpdateReq)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error scheduling patch `%s`: %s", patchID, err))
	}
	scheduledPatch, err := r.sc.FindPatchById(versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting scheduled patch `%s`: %s", patchID, err))
	}
	return scheduledPatch, nil
}

func (r *mutationResolver) SchedulePatchTasks(ctx context.Context, patchID string) (*string, error) {
	modifications := VersionModifications{
		Action: SetActive,
		Active: true,
		Abort:  false,
	}
	err := ModifyVersionHandler(ctx, r.sc, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *mutationResolver) UnschedulePatchTasks(ctx context.Context, patchID string, abort bool) (*string, error) {
	modifications := VersionModifications{
		Action: SetActive,
		Active: false,
		Abort:  abort,
	}
	err := ModifyVersionHandler(ctx, r.sc, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

// ScheduleUndispatchedBaseTasks only allows scheduling undispatched base tasks for tasks that are failing on the current patch
func (r *mutationResolver) ScheduleUndispatchedBaseTasks(ctx context.Context, patchID string) ([]*restModel.APITask, error) {
	opts := data.TaskFilterOptions{
		Statuses:              evergreen.TaskFailureStatuses,
		IncludeExecutionTasks: true,
		IncludeBaseTasks:      false,
	}
	tasks, _, err := r.sc.FindTasksByVersion(patchID, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Could not fetch tasks for patch: %s ", err.Error()))
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
					return nil, InternalServerError.Send(ctx, fmt.Sprintf("Experienced an error trying to find the generator task: %s", err.Error()))
				}
				if generatorTask != nil {
					baseGeneratorTask, _ := generatorTask.FindTaskOnBaseCommit()
					// If baseGeneratorTask is nil then it didn't exist on the base task and we can't do anything
					if baseGeneratorTask != nil && baseGeneratorTask.Status == evergreen.TaskUndispatched {
						err = baseGeneratorTask.SetGeneratedTasksToActivate(t.BuildVariant, t.DisplayName)
						if err != nil {
							return nil, InternalServerError.Send(ctx, fmt.Sprintf("Could not activate generated task: %s", err.Error()))
						}
						tasksToSchedule[baseGeneratorTask.Id] = true

					}
				}
			} else if baseTask.Status == evergreen.TaskUndispatched {
				tasksToSchedule[baseTask.Id] = true
			}

		}
	}

	for taskId := range tasksToSchedule {
		task, err := SetScheduled(ctx, r.sc, taskId, true)
		if err != nil {
			return nil, err
		}
		scheduledTasks = append(scheduledTasks, task)
	}
	// sort scheduledTasks by display name to guarantee the order of the tasks
	sort.Slice(scheduledTasks, func(i, j int) bool {
		return *scheduledTasks[i].DisplayName < *scheduledTasks[j].DisplayName
	})

	return scheduledTasks, nil
}

func (r *mutationResolver) RestartPatch(ctx context.Context, patchID string, abort bool, taskIds []string) (*string, error) {
	if len(taskIds) == 0 {
		return nil, InputValidationError.Send(ctx, "`taskIds` array is empty. You must provide at least one task id")
	}
	modifications := VersionModifications{
		Action:  Restart,
		Abort:   abort,
		TaskIds: taskIds,
	}
	err := ModifyVersionHandler(ctx, r.sc, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *mutationResolver) RestartVersions(ctx context.Context, patchID string, abort bool, versionsToRestart []*model.VersionToRestart) ([]*restModel.APIVersion, error) {
	if len(versionsToRestart) == 0 {
		return nil, InputValidationError.Send(ctx, "No versions provided. You must provide at least one version to restart")
	}
	modifications := VersionModifications{
		Action:            Restart,
		Abort:             abort,
		VersionsToRestart: versionsToRestart,
	}
	err := ModifyVersionHandler(ctx, r.sc, patchID, modifications)
	if err != nil {
		return nil, err
	}
	versions := []*restModel.APIVersion{}
	for _, version := range versionsToRestart {
		if version.VersionId != nil {
			v, versionErr := r.sc.FindVersionById(*version.VersionId)
			if versionErr != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding version by id %s: %s", *version.VersionId, versionErr.Error()))
			}
			if v == nil {
				return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find version with id %s", *version.VersionId))
			}
			apiVersion := restModel.APIVersion{}
			if err = apiVersion.BuildFromService(v); err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APIVersion from service for `%s`: %s", *version.VersionId, err.Error()))
			}
			versions = append(versions, &apiVersion)
		}
	}

	return versions, nil
}

func (r *mutationResolver) SetPatchPriority(ctx context.Context, patchID string, priority int) (*string, error) {
	modifications := VersionModifications{
		Action:   SetPriority,
		Priority: int64(priority),
	}
	err := ModifyVersionHandler(ctx, r.sc, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *mutationResolver) EnqueuePatch(ctx context.Context, patchID string, commitMessage *string) (*restModel.APIPatch, error) {
	user := MustHaveUser(ctx)

	existingPatch, err := r.sc.FindPatchById(patchID)
	if err != nil {
		gimletErr, ok := err.(gimlet.ErrorResponse)
		if ok {
			return nil, mapHTTPStatusToGqlError(ctx, gimletErr.StatusCode, err)
		}
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error getting patch '%s'", patchID))
	}

	if !hasEnqueuePatchPermission(user, existingPatch) {
		return nil, Forbidden.Send(ctx, "can't enqueue another user's patch")
	}

	if commitMessage == nil {
		commitMessage = existingPatch.Description
	}

	newPatch, err := r.sc.CreatePatchForMerge(ctx, patchID, utility.FromStringPtr(commitMessage))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error creating new patch: %s", err.Error()))
	}
	item := restModel.APICommitQueueItem{
		Issue:   newPatch.Id,
		PatchId: newPatch.Id,
		Source:  utility.ToStringPtr(commitqueue.SourceDiff)}
	_, err = r.sc.EnqueueItem(utility.FromStringPtr(newPatch.ProjectId), item, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error enqueuing new patch: %s", err.Error()))
	}

	return newPatch, nil
}

func (r *mutationResolver) ScheduleTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	task, err := SetScheduled(ctx, r.sc, taskID, true)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (r *mutationResolver) UnscheduleTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	task, err := SetScheduled(ctx, r.sc, taskID, false)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (r *mutationResolver) AbortTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	t, err := r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	user := gimlet.GetUser(ctx).DisplayName()
	err = model.AbortTask(taskID, user)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error aborting task %s: %s", taskID, err.Error()))
	}
	t, err = r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *t)
	return apiTask, err
}

func (r *mutationResolver) RestartTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	usr := MustHaveUser(ctx)
	username := usr.Username()
	if err := model.TryResetTask(taskID, username, evergreen.UIPackage, nil); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error restarting task %s: %s", taskID, err.Error()))
	}
	t, err := task.FindOneIdAndExecutionWithDisplayStatus(taskID, nil)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *t)
	return apiTask, err
}

// EditAnnotationNote updates the note for the annotation, assuming it hasn't been updated in the meantime.
func (r *mutationResolver) EditAnnotationNote(ctx context.Context, taskID string, execution int, originalMessage, newMessage string) (bool, error) {
	usr := MustHaveUser(ctx)
	if err := annotations.UpdateAnnotationNote(taskID, execution, originalMessage, newMessage, usr.Username()); err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("couldn't update note: %s", err.Error()))
	}
	return true, nil
}

// MoveAnnotationIssue moves an issue for the annotation. If isIssue is set, it removes the issue from Issues and adds it
// to Suspected Issues, otherwise vice versa.
func (r *mutationResolver) MoveAnnotationIssue(ctx context.Context, taskID string, execution int, apiIssue restModel.APIIssueLink, isIssue bool) (bool, error) {
	usr := MustHaveUser(ctx)
	issue := restModel.APIIssueLinkToService(apiIssue)
	if isIssue {
		if err := annotations.MoveIssueToSuspectedIssue(taskID, execution, *issue, usr.Username()); err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("couldn't move issue to suspected issues: %s", err.Error()))
		}
		return true, nil
	} else {
		if err := annotations.MoveSuspectedIssueToIssue(taskID, execution, *issue, usr.Username()); err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("couldn't move issue to suspected issues: %s", err.Error()))
		}
		return true, nil
	}
}

// AddAnnotationIssue adds to the annotation for that taskID/execution.
// If isIssue is set, it adds to Issues, otherwise it adds to Suspected Issues.
func (r *mutationResolver) AddAnnotationIssue(ctx context.Context, taskID string, execution int,
	apiIssue restModel.APIIssueLink, isIssue bool) (bool, error) {
	usr := MustHaveUser(ctx)
	issue := restModel.APIIssueLinkToService(apiIssue)
	if err := util.CheckURL(issue.URL); err != nil {
		return false, InputValidationError.Send(ctx, fmt.Sprintf("issue does not have valid URL: %s", err.Error()))
	}
	if isIssue {
		if err := annotations.AddIssueToAnnotation(taskID, execution, *issue, usr.Username()); err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("couldn't add issue: %s", err.Error()))
		}
		return true, nil
	} else {
		if err := annotations.AddSuspectedIssueToAnnotation(taskID, execution, *issue, usr.Username()); err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("couldn't add suspected issue: %s", err.Error()))
		}
		return true, nil
	}
}

// RemoveAnnotationIssue adds to the annotation for that taskID/execution.
// If isIssue is set, it adds to Issues, otherwise it adds to Suspected Issues.
func (r *mutationResolver) RemoveAnnotationIssue(ctx context.Context, taskID string, execution int,
	apiIssue restModel.APIIssueLink, isIssue bool) (bool, error) {
	issue := restModel.APIIssueLinkToService(apiIssue)
	if isIssue {
		if err := annotations.RemoveIssueFromAnnotation(taskID, execution, *issue); err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("couldn't delete issue: %s", err.Error()))
		}
		return true, nil
	} else {
		if err := annotations.RemoveSuspectedIssueFromAnnotation(taskID, execution, *issue); err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("couldn't delete suspected issue: %s", err.Error()))
		}
		return true, nil
	}
}

func (r *mutationResolver) RemoveItemFromCommitQueue(ctx context.Context, commitQueueID string, issue string) (*string, error) {
	result, err := r.sc.CommitQueueRemoveItem(commitQueueID, issue, gimlet.GetUser(ctx).DisplayName())
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error removing item %s from commit queue %s: %s",
			issue, commitQueueID, err.Error()))
	}
	if result == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("couldn't remove item %s from commit queue %s", issue, commitQueueID))
	}

	return &issue, nil
}

func (r *mutationResolver) ClearMySubscriptions(ctx context.Context) (int, error) {
	usr := MustHaveUser(ctx)
	username := usr.Username()
	subs, err := r.sc.GetSubscriptions(username, event.OwnerTypePerson)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("Error retreiving subscriptions %s", err.Error()))
	}
	subIds := []string{}
	for _, sub := range subs {
		if sub.ID != nil {
			subIds = append(subIds, *sub.ID)
		}
	}
	err = r.sc.DeleteSubscriptions(username, subIds)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("Error deleting subscriptions %s", err.Error()))
	}

	return len(subIds), nil
}

func (r *mutationResolver) SaveSubscription(ctx context.Context, subscription restModel.APISubscription) (bool, error) {
	usr := MustHaveUser(ctx)
	username := usr.Username()
	idType, id, err := getResourceTypeAndIdFromSubscriptionSelectors(ctx, subscription.Selectors)
	if err != nil {
		return false, err
	}
	switch idType {
	case "task":
		t, taskErr := r.sc.FindTaskById(id)
		if taskErr != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", id, taskErr.Error()))
		}
		if t == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", id))
		}
		break
	case "build":
		b, buildErr := r.sc.FindBuildById(id)
		if buildErr != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding build by id %s: %s", id, buildErr.Error()))
		}
		if b == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find build with id %s", id))
		}
		break
	case "version":
		v, versionErr := r.sc.FindVersionById(id)
		if versionErr != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding version by id %s: %s", id, versionErr.Error()))
		}
		if v == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find version with id %s", id))
		}
		break
	case "project":
		p, projectErr := r.sc.FindProjectById(id, false)
		if projectErr != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding project by id %s: %s", id, projectErr.Error()))
		}
		if p == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find project with id %s", id))
		}
		break
	default:
		return false, InputValidationError.Send(ctx, "Selectors do not indicate a target version, build, project, or task ID")
	}
	err = r.sc.SaveSubscriptions(username, []restModel.APISubscription{subscription}, false)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("error saving subscription: %s", err.Error()))
	}
	return true, nil
}

func (r *mutationResolver) UpdateUserSettings(ctx context.Context, userSettings *restModel.APIUserSettings) (bool, error) {
	usr := MustHaveUser(ctx)

	updatedUserSettings, err := restModel.UpdateUserSettings(ctx, usr, *userSettings)
	if err != nil {
		return false, InternalServerError.Send(ctx, err.Error())
	}
	err = r.sc.UpdateSettings(usr, *updatedUserSettings)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("Error saving userSettings : %s", err.Error()))
	}
	return true, nil
}

func (r *mutationResolver) RestartJasper(ctx context.Context, hostIds []string) (int, error) {
	user := MustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(user, hostIds)
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetRestartJasperCallback(ctx, evergreen.GetEnvironment(), user.Username()))
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, errors.Errorf("error marking selected hosts as needing Jasper service restarted: %s", err.Error()))
	}

	return hostsUpdated, nil
}

func (r *mutationResolver) UpdateHostStatus(ctx context.Context, hostIds []string, status string, notes *string) (int, error) {
	user := MustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(user, hostIds)
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	rq := evergreen.GetEnvironment().RemoteQueue()

	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetUpdateHostStatusCallback(ctx, evergreen.GetEnvironment(), rq, status, *notes, user))
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	return hostsUpdated, nil
}

func (r *mutationResolver) CreatePublicKey(ctx context.Context, publicKeyInput PublicKeyInput) ([]*restModel.APIPubKey, error) {
	err := savePublicKey(ctx, publicKeyInput)
	if err != nil {
		return nil, err
	}
	myPublicKeys := getMyPublicKeys(ctx)
	return myPublicKeys, nil
}

func (r *mutationResolver) RemovePublicKey(ctx context.Context, keyName string) ([]*restModel.APIPubKey, error) {
	if !doesPublicKeyNameAlreadyExist(ctx, keyName) {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("Error deleting public key. Provided key name, %s, does not exist.", keyName))
	}
	err := MustHaveUser(ctx).DeletePublicKey(keyName)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error deleting public key: %s", err.Error()))
	}
	myPublicKeys := getMyPublicKeys(ctx)
	return myPublicKeys, nil
}

func (r *mutationResolver) RemoveVolume(ctx context.Context, volumeID string) (bool, error) {
	success, _, gqlErr, err := DeleteVolume(ctx, volumeID)
	if err != nil {
		return false, gqlErr.Send(ctx, err.Error())
	}
	return success, nil
}

func (r *mutationResolver) UpdatePublicKey(ctx context.Context, targetKeyName string, updateInfo PublicKeyInput) ([]*restModel.APIPubKey, error) {
	if !doesPublicKeyNameAlreadyExist(ctx, targetKeyName) {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("Error updating public key. The target key name, %s, does not exist.", targetKeyName))
	}
	if updateInfo.Name != targetKeyName && doesPublicKeyNameAlreadyExist(ctx, updateInfo.Name) {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("Error updating public key. The updated key name, %s, already exists.", targetKeyName))
	}
	err := verifyPublicKey(ctx, updateInfo)
	if err != nil {
		return nil, err
	}
	usr := MustHaveUser(ctx)
	err = usr.UpdatePublicKey(targetKeyName, updateInfo.Name, updateInfo.Key)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error updating public key, %s: %s", targetKeyName, err.Error()))
	}
	myPublicKeys := getMyPublicKeys(ctx)
	return myPublicKeys, nil
}

func (r *queryResolver) User(ctx context.Context, userIdParam *string) (*restModel.APIDBUser, error) {
	usr := MustHaveUser(ctx)
	var err error
	if userIdParam != nil {
		usr, err = user.FindOneById(*userIdParam)
		if err != nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Error getting user from user ID: %s", err.Error()))
		}
		if usr == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find user from user ID"))
		}
	}
	displayName := usr.DisplayName()
	userID := usr.Username()
	email := usr.Email()
	user := restModel.APIDBUser{
		DisplayName:  &displayName,
		UserID:       &userID,
		EmailAddress: &email,
	}
	return &user, nil
}

func (r *userResolver) Patches(ctx context.Context, obj *restModel.APIDBUser, patchesInput PatchesInput) (*Patches, error) {
	patches, count, err := r.sc.FindPatchesByUserPatchNameStatusesCommitQueue(*obj.UserID, patchesInput.PatchName, patchesInput.Statuses, patchesInput.IncludeCommitQueue, patchesInput.Page, patchesInput.Limit)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	patchPointers := []*restModel.APIPatch{}
	for i := range patches {
		patchPointers = append(patchPointers, &patches[i])
	}

	return &Patches{Patches: patchPointers, FilteredPatchCount: *count}, nil
}

func (r *queryResolver) InstanceTypes(ctx context.Context) ([]string, error) {
	config, err := evergreen.GetConfig()
	if err != nil {
		return nil, InternalServerError.Send(ctx, "unable to retrieve server config")
	}
	return config.Providers.AWS.AllowedInstanceTypes, nil
}

type taskResolver struct{ *Resolver }

func (r *taskResolver) DisplayTask(ctx context.Context, obj *restModel.APITask) (*restModel.APITask, error) {
	t, err := r.sc.FindTaskById(*obj.Id)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find task with id: %s", *obj.Id))
	}
	dt, err := t.GetDisplayTask()
	if dt == nil || err != nil {
		return nil, nil
	}
	apiTask := &restModel.APITask{}
	if err = apiTask.BuildFromService(dt); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert display task: %s to APITask", dt.Id))
	}
	return apiTask, nil
}

func (r *taskResolver) EstimatedStart(ctx context.Context, obj *restModel.APITask) (*restModel.APIDuration, error) {
	i, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while converting task %s to service", *obj.Id))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *obj.Id))
	}
	start, err := model.GetEstimatedStartTime(*t)
	if err != nil {
		return nil, InternalServerError.Send(ctx, "error getting estimated start time")
	}
	duration := restModel.NewAPIDuration(start)
	return &duration, nil
}

func (r *taskResolver) TotalTestCount(ctx context.Context, obj *restModel.APITask) (int, error) {
	if obj.HasCedarResults {
		opts := apimodels.GetCedarTestResultsOptions{
			BaseURL:     evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:      utility.FromStringPtr(obj.Id),
			Execution:   utility.ToIntPtr(obj.Execution),
			DisplayTask: obj.DisplayOnly,
		}
		stats, err := apimodels.GetCedarTestResultsStats(ctx, opts)
		if err != nil {
			return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting test count: %s", err))
		}

		return stats.TotalCount, nil
	}

	testCount, err := r.sc.GetTestCountByTaskIdAndFilters(*obj.Id, "", nil, obj.Execution)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting test count: %s", err))
	}

	return testCount, nil
}

func (r *taskResolver) FailedTestCount(ctx context.Context, obj *restModel.APITask) (int, error) {
	if obj.HasCedarResults {
		opts := apimodels.GetCedarTestResultsOptions{
			BaseURL:     evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:      utility.FromStringPtr(obj.Id),
			Execution:   utility.ToIntPtr(obj.Execution),
			DisplayTask: obj.DisplayOnly,
		}
		stats, err := apimodels.GetCedarTestResultsStats(ctx, opts)
		if err != nil {
			return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting failed test count: %s", err))
		}

		return stats.FailedCount, nil
	}

	failedTestCount, err := r.sc.GetTestCountByTaskIdAndFilters(*obj.Id, "", []string{evergreen.TestFailedStatus}, obj.Execution)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting failed test count: %s", err))
	}

	return failedTestCount, nil
}

// TODO: deprecated
func (r *taskResolver) PatchMetadata(ctx context.Context, obj *restModel.APITask) (*PatchMetadata, error) {
	version, err := r.sc.FindVersionById(*obj.Version)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error retrieving version %s: %s", *obj.Version, err.Error()))
	}
	patchMetadata := PatchMetadata{
		Author:  version.Author,
		PatchID: version.Id,
	}
	return &patchMetadata, nil
}

func (r *taskResolver) BaseTaskMetadata(ctx context.Context, at *restModel.APITask) (*BaseTaskMetadata, error) {
	i, err := at.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *at.Id, err.Error()))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *at.Id))
	}
	baseTask, err := t.FindTaskOnBaseCommit()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding task %s on base commit", *at.Id))
	}
	if baseTask == nil {
		return nil, nil
	}
	config, err := evergreen.GetConfig()
	if err != nil {
		return nil, InternalServerError.Send(ctx, "unable to retrieve server config")
	}

	dur := restModel.NewAPIDuration(baseTask.TimeTaken)
	baseTaskMetadata := BaseTaskMetadata{
		BaseTaskLink:     fmt.Sprintf("%s/task/%s", config.Ui.Url, baseTask.Id),
		BaseTaskDuration: &dur,
	}
	if baseTask.TimeTaken == 0 {
		baseTaskMetadata.BaseTaskDuration = nil
	}
	return &baseTaskMetadata, nil
}

func (r *taskResolver) SpawnHostLink(ctx context.Context, at *restModel.APITask) (*string, error) {
	host, err := host.FindOne(host.ById(*at.HostId))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding host for task %s", *at.Id))
	}
	if host == nil {
		return nil, nil
	}
	if host.Distro.SpawnAllowed && utility.StringSliceContains(evergreen.ProviderUserSpawnable, host.Distro.Provider) {
		link := fmt.Sprintf("%s/spawn?distro_id=%s&task_id=%s", evergreen.GetEnvironment().Settings().Ui.Url, host.Distro.Id, *at.Id)
		return &link, nil
	}
	return nil, nil
}

func (r *taskResolver) PatchNumber(ctx context.Context, obj *restModel.APITask) (*int, error) {
	order := obj.Order
	return &order, nil
}

func (r *taskResolver) CanRestart(ctx context.Context, obj *restModel.APITask) (bool, error) {
	canRestart, err := canRestartTask(ctx, obj)
	if err != nil {
		return false, err
	}
	return *canRestart, nil
}

func (r *taskResolver) CanAbort(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return *obj.Status == evergreen.TaskDispatched || *obj.Status == evergreen.TaskStarted, nil
}

func (r *taskResolver) CanSchedule(ctx context.Context, obj *restModel.APITask) (bool, error) {
	canRestart, err := canRestartTask(ctx, obj)
	if err != nil {
		return false, err
	}
	return *canRestart == false && !obj.Aborted, nil
}

func (r *taskResolver) CanUnschedule(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return obj.Activated && *obj.Status == evergreen.TaskUndispatched, nil
}

func (r *taskResolver) CanSetPriority(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return *obj.Status == evergreen.TaskUndispatched, nil
}

func (r *taskResolver) Status(ctx context.Context, obj *restModel.APITask) (string, error) {
	return *obj.DisplayStatus, nil
}

func (r *taskResolver) LatestExecution(ctx context.Context, obj *restModel.APITask) (int, error) {
	return task.GetLatestExecution(*obj.Id)
}

func (r *taskResolver) GeneratedByName(ctx context.Context, obj *restModel.APITask) (*string, error) {
	if obj.GeneratedBy == "" {
		return nil, nil
	}
	generator, err := task.FindOneIdWithFields(obj.GeneratedBy, task.DisplayNameKey)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("unable to find generator: %s", err.Error()))
	}
	if generator == nil {
		return nil, nil
	}
	name := generator.DisplayName

	return &name, nil
}

func (r *taskResolver) IsPerfPluginEnabled(ctx context.Context, obj *restModel.APITask) (bool, error) {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return false, err
	}
	if flags.PluginAdminPageDisabled {
		return model.IsPerfEnabledForProject(*obj.ProjectId), nil
	} else {
		var perfPlugin *plugin.PerfPlugin
		pRef, err := r.sc.FindProjectById(*obj.ProjectId, false)
		if err != nil {
			return false, err
		}
		if perfPluginSettings, exists := evergreen.GetEnvironment().Settings().Plugins[perfPlugin.Name()]; exists {
			err := mapstructure.Decode(perfPluginSettings, &perfPlugin)
			if err != nil {
				return false, err
			}
			for _, projectName := range perfPlugin.Projects {
				if projectName == pRef.Id || projectName == pRef.Identifier {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (r *taskResolver) MinQueuePosition(ctx context.Context, obj *restModel.APITask) (int, error) {
	position, err := model.FindMinimumQueuePositionForTask(*obj.Id)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("error queue position for task: %s", err.Error()))
	}
	if position < 0 {
		return 0, nil
	}
	return position, nil
}
func (r *taskResolver) BaseStatus(ctx context.Context, obj *restModel.APITask) (*string, error) {
	i, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *obj.Id, err.Error()))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *obj.Id))
	}
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding task %s on base commit", *obj.Id))
	}
	baseStatus := t.BaseTask.Status
	if baseStatus == "" {
		return nil, nil
	}
	return &baseStatus, nil
}

func (r *taskResolver) BaseTask(ctx context.Context, obj *restModel.APITask) (*restModel.APITask, error) {
	i, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *obj.Id, err.Error()))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *obj.Id))
	}
	baseTaskID := t.BaseTask.Id
	if baseTaskID == "" {
		return nil, nil
	}
	baseTask, err := r.sc.FindTaskById(baseTaskID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding task %s on base commit", *obj.Id))
	}
	if baseTask == nil {
		return nil, nil
	}
	apiTask := &restModel.APITask{}
	err = apiTask.BuildFromService(baseTask)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert baseTask %s to APITask : %s", baseTask.Id, err))
	}
	return apiTask, nil
}
func (r *taskResolver) ExecutionTasksFull(ctx context.Context, obj *restModel.APITask) ([]*restModel.APITask, error) {
	if len(obj.ExecutionTasks) == 0 {
		return nil, nil
	}
	tasks, err := task.FindByExecutionTasksAndMaxExecution(obj.ExecutionTasks, obj.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding execution tasks for task: %s : %s", *obj.Id, err.Error()))
	}
	apiTasks := []*restModel.APITask{}
	for _, t := range tasks {
		apiTask := &restModel.APITask{}
		err = apiTask.BuildFromService(&t)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert task %s to APITask : %s", t.Id, err.Error()))
		}
		apiTasks = append(apiTasks, apiTask)
	}
	return apiTasks, nil
}

func (r *taskResolver) BuildVariantDisplayName(ctx context.Context, obj *restModel.APITask) (*string, error) {
	if obj.BuildId == nil {
		return nil, nil
	}
	buildID := utility.FromStringPtr(obj.BuildId)
	b, err := build.FindOneId(buildID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to find build id: %s for task: %s", buildID, utility.FromStringPtr(obj.Id)))
	}
	displayName := b.DisplayName
	return &displayName, nil

}

func (r *taskResolver) VersionMetadata(ctx context.Context, obj *restModel.APITask) (*restModel.APIVersion, error) {
	v, err := r.sc.FindVersionById(utility.FromStringPtr(obj.Version))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to find version id: %s for task: %s", *obj.Version, utility.FromStringPtr(obj.Id)))
	}
	apiVersion := &restModel.APIVersion{}
	if err = apiVersion.BuildFromService(v); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert version: %s to APIVersion", v.Id))
	}
	return apiVersion, nil
}

func (r *queryResolver) BuildBaron(ctx context.Context, taskID string, exec int) (*BuildBaron, error) {
	execString := strconv.Itoa(exec)

	searchReturnInfo, bbConfig, err := GetSearchReturnInfo(taskID, execString)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}

	return &BuildBaron{
		SearchReturnInfo:     searchReturnInfo,
		BuildBaronConfigured: bbConfig.ProjectFound && bbConfig.SearchConfigured,
	}, nil
}

func (r *mutationResolver) BbCreateTicket(ctx context.Context, taskID string, execution *int) (bool, error) {
	taskNotFound, err := BbFileTicket(ctx, taskID, *execution)
	successful := true

	if err != nil {
		return !successful, InternalServerError.Send(ctx, err.Error())
	}
	if taskNotFound {
		return !successful, ResourceNotFound.Send(ctx, fmt.Sprintf("could not find task '%s'", taskID))
	}

	return successful, nil
}

func (r *queryResolver) BbGetCreatedTickets(ctx context.Context, taskID string) ([]*thirdparty.JiraTicket, error) {
	createdTickets, err := BbGetCreatedTicketsPointers(taskID)
	if err != nil {
		return nil, err
	}

	return createdTickets, nil
}

func (r *queryResolver) TaskNamesForBuildVariant(ctx context.Context, projectId string, buildVariant string) ([]string, error) {
	buildVariantTasks, err := task.FindTaskNamesByBuildVariant(projectId, buildVariant)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while getting tasks for '%s': %s", buildVariant, err.Error()))
	}
	if buildVariantTasks == nil {
		return []string{}, nil
	}
	return buildVariantTasks, nil
}

func (r *queryResolver) BuildVariantsForTaskName(ctx context.Context, projectId string, taskName string) ([]*task.BuildVariantTuple, error) {
	taskBuildVariants, err := task.FindUniqueBuildVariantNamesByTask(projectId, taskName)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while getting build variant tasks for task '%s': %s", taskName, err.Error()))
	}
	if taskBuildVariants == nil {
		return nil, nil
	}
	return taskBuildVariants, nil

}

// Will return an array of activated and unactivated versions
func (r *queryResolver) MainlineCommits(ctx context.Context, options MainlineCommitsOptions, buildVariantOptions *BuildVariantOptions) (*MainlineCommits, error) {
	projectId, err := model.GetIdForProject(options.ProjectID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project with id: %s", options.ProjectID))
	}
	limit := model.DefaultMainlineCommitVersionLimit
	if utility.FromIntPtr(options.Limit) != 0 {
		limit = utility.FromIntPtr(options.Limit)
	}
	opts := model.MainlineCommitVersionOptions{
		Limit:           limit,
		SkipOrderNumber: utility.FromIntPtr(options.SkipOrderNumber),
	}

	versions, err := model.GetMainlineCommitVersionsWithOptions(projectId, opts)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Error getting activated versions: %s", err.Error()))
	}

	var mainlineCommits MainlineCommits
	matchingVersionCount := 0

	// We only want to return the PrevPageOrderNumber if a user is not on the first page
	if options.SkipOrderNumber != nil {
		prevPageCommit, err := model.GetPreviousPageCommitOrderNumber(projectId, utility.FromIntPtr(options.SkipOrderNumber), limit)

		if err != nil {
			// This shouldn't really happen, but if it does, we should return an error and log it
			grip.Warning(message.WrapError(err, message.Fields{
				"message":    "Error getting most recent version",
				"project_id": projectId,
			}))
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting most recent mainline commit: %s", err.Error()))
		}

		if prevPageCommit != nil {
			mainlineCommits.PrevPageOrderNumber = prevPageCommit
		}
	}

	index := 0
	versionsCheckedCount := 0

	// We will loop through each version returned from GetMainlineCommitVersionsWithOptions and see if there is a commit that matches the filter parameters (if any).
	// If there is a match, we will add it to the array of versions to be returned to the user.
	// If there are not enough matches to satisfy our limit, we will call GetMainlineCommitVersionsWithOptions again with the next order number to check and repeat the process.
	for matchingVersionCount < limit {
		// If we no longer have any more versions to check break out and return what we have.
		if len(versions) == 0 {
			break
		}
		// If we have checked more versions than the MaxMainlineCommitVersionLimit then break out and return what we have.
		if versionsCheckedCount >= model.MaxMainlineCommitVersionLimit {
			// Return an error if we did not find any versions that match.
			if matchingVersionCount == 0 {
				return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Matching version not found in %d most recent versions", model.MaxMainlineCommitVersionLimit))
			}
			break
		}
		versionsCheckedCount++
		v := versions[index]
		apiVersion := restModel.APIVersion{}
		err = apiVersion.BuildFromService(&v)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APIVersion from service: %s", err.Error()))
		}

		// If the version was created before we started caching activation status we must manually verify it and cache that value.
		if v.Activated == nil {
			err = setVersionActivationStatus(r.sc, &v)
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error setting version activation status: %s", err.Error()))
			}
		}
		mainlineCommitVersion := MainlineCommitVersion{}
		shouldCollapse := false
		if !utility.FromBoolPtr(v.Activated) {
			shouldCollapse = true
		} else if buildVariantOptions.isPopulated() {
			opts := task.HasMatchingTasksOptions{
				TaskNames: buildVariantOptions.Tasks,
				Variants:  buildVariantOptions.Variants,
				Statuses:  buildVariantOptions.Statuses,
			}
			hasTasks, err := task.HasMatchingTasks(v.Id, opts)
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error checking if version has tasks: %s", err.Error()))
			}
			if !hasTasks {
				shouldCollapse = true
			}
		}
		// If a version matches our filter criteria we append it directly to our returned list of mainlineCommits
		if !shouldCollapse {
			matchingVersionCount++
			mainlineCommits.NextPageOrderNumber = utility.ToIntPtr(v.RevisionOrderNumber)
			mainlineCommitVersion.Version = &apiVersion

		} else {
			// If a version does not match our filter criteria roll up all the unactivated versions that are sequentially near each other into a single MainlineCommitVersion,
			// and then append it to our returned list.
			// If we have any versions already we should check the most recent one first otherwise create a new one
			if len(mainlineCommits.Versions) > 0 {
				lastMainlineCommit := mainlineCommits.Versions[len(mainlineCommits.Versions)-1]

				// If the previous mainlineCommit contains rolled up unactivated versions append the latest RolledUp unactivated version
				if lastMainlineCommit.RolledUpVersions != nil {
					lastMainlineCommit.RolledUpVersions = append(lastMainlineCommit.RolledUpVersions, &apiVersion)
				} else {
					mainlineCommitVersion.RolledUpVersions = []*restModel.APIVersion{&apiVersion}
				}

			} else {
				mainlineCommitVersion.RolledUpVersions = []*restModel.APIVersion{&apiVersion}

			}

		}

		// Only add a mainlineCommit if a new one was added and it's not a modified existing RolledUpVersion
		if mainlineCommitVersion.Version != nil || mainlineCommitVersion.RolledUpVersions != nil {
			mainlineCommits.Versions = append(mainlineCommits.Versions, &mainlineCommitVersion)
		}
		index++
		// If we have exhausted all of our versions we should fetch some more.
		if index == len(versions) && matchingVersionCount < limit {
			skipOrderNumber := versions[len(versions)-1].RevisionOrderNumber
			opts := model.MainlineCommitVersionOptions{
				Limit:           limit,
				SkipOrderNumber: skipOrderNumber,
			}

			versions, err = model.GetMainlineCommitVersionsWithOptions(projectId, opts)
			if err != nil {
				return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Error getting activated versions: %s", err.Error()))
			}
			index = 0
		}
	}

	return &mainlineCommits, nil
}

type versionResolver struct{ *Resolver }

func (r *versionResolver) Manifest(ctx context.Context, v *restModel.APIVersion) (*Manifest, error) {
	m, err := manifest.FindFromVersion(*v.Id, *v.Project, *v.Revision, *v.Requester)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error fetching manifest for version %s : %s", *v.Id, err.Error()))
	}
	if m == nil {
		return nil, nil
	}
	versionManifest := Manifest{
		ID:              m.Id,
		Revision:        m.Revision,
		Project:         m.ProjectName,
		Branch:          m.Branch,
		IsBase:          m.IsBase,
		ModuleOverrides: m.ModuleOverrides,
	}
	modules := map[string]interface{}{}
	for key, module := range m.Modules {
		modules[key] = module
	}
	versionManifest.Modules = modules

	return &versionManifest, nil
}
func (r *versionResolver) TaskStatuses(ctx context.Context, v *restModel.APIVersion) ([]string, error) {
	defaultSort := []task.TasksSortOrder{
		{Key: task.DisplayNameKey, Order: 1},
	}
	opts := data.TaskFilterOptions{
		Sorts:            defaultSort,
		IncludeBaseTasks: false,
		FieldsToProject:  []string{task.DisplayStatusKey},
	}
	tasks, _, err := r.sc.FindTasksByVersion(*v.Id, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting version tasks: %s", err.Error()))
	}
	return getAllTaskStatuses(tasks), nil
}

func (r *versionResolver) BaseTaskStatuses(ctx context.Context, v *restModel.APIVersion) ([]string, error) {
	baseVersion, err := model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(*v.Project, *v.Revision).WithFields(model.VersionIdentifierKey))
	if baseVersion == nil || err != nil {
		return nil, nil
	}
	defaultSort := []task.TasksSortOrder{
		{Key: task.DisplayNameKey, Order: 1},
	}
	opts := data.TaskFilterOptions{
		Sorts:            defaultSort,
		IncludeBaseTasks: false,
		FieldsToProject:  []string{task.DisplayStatusKey},
	}
	tasks, _, err := r.sc.FindTasksByVersion(baseVersion.Id, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting version tasks: %s", err.Error()))
	}
	return getAllTaskStatuses(tasks), nil
}

// Returns task status counts (a mapping between status and the number of tasks with that status) for a version.
func (r *versionResolver) TaskStatusCounts(ctx context.Context, v *restModel.APIVersion, options *BuildVariantOptions) ([]*task.StatusCount, error) {
	opts := task.GetTasksByVersionOptions{
		IncludeBaseTasks:      false,
		IncludeExecutionTasks: false,
		TaskNames:             options.Tasks,
		Variants:              options.Variants,
		Statuses:              options.Statuses,
	}
	stats, err := task.GetTaskStatsByVersion(*v.Id, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting version task stats: %s", err.Error()))
	}

	return stats, nil
}

// Returns grouped build variants for a version. Will not return build variants for unactivated versions
func (r *versionResolver) BuildVariants(ctx context.Context, v *restModel.APIVersion, options *BuildVariantOptions) ([]*GroupedBuildVariant, error) {
	// If activated is nil in the db we should resolve it and cache it for subsequent queries. There is a very low likely hood of this field being hit
	if v.Activated == nil {
		defaultSort := []task.TasksSortOrder{
			{Key: task.DisplayNameKey, Order: 1},
		}
		opts := data.TaskFilterOptions{
			Sorts:            defaultSort,
			IncludeBaseTasks: true,
		}
		tasks, _, err := r.sc.FindTasksByVersion(*v.Id, opts)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error fetching tasks for version %s : %s", *v.Id, err.Error()))
		}
		version, err := model.VersionFindOne(model.VersionById(*v.Id))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error fetching version: %s : %s", *v.Id, err.Error()))
		}
		if !task.AnyActiveTasks(tasks) {
			err = version.SetNotActivated()
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error updating version activated status for %s, %s", *v.Id, err.Error()))
			}
		} else {
			err = version.SetActivated()
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error updating version activated status for %s, %s", *v.Id, err.Error()))
			}
		}
		v.Activated = version.Activated
	}

	if !utility.FromBoolPtr(v.Activated) {
		return nil, nil
	}
	groupedBuildVariants, err := generateBuildVariants(r.sc, *v.Id, options.Variants, options.Tasks, options.Statuses)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error generating build variants for version %s : %s", *v.Id, err.Error()))
	}
	return groupedBuildVariants, nil
}

func (r *versionResolver) IsPatch(ctx context.Context, v *restModel.APIVersion) (bool, error) {
	return evergreen.IsPatchRequester(*v.Requester), nil
}

func (r *versionResolver) Patch(ctx context.Context, v *restModel.APIVersion) (*restModel.APIPatch, error) {
	if !evergreen.IsPatchRequester(*v.Requester) {
		return nil, nil
	}
	apiPatch, err := r.sc.FindPatchById(*v.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Couldn't find a patch with id: `%s` %s", *v.Id, err.Error()))
	}
	return apiPatch, nil
}

func (r *versionResolver) ChildVersions(ctx context.Context, v *restModel.APIVersion) ([]*restModel.APIVersion, error) {
	if !evergreen.IsPatchRequester(*v.Requester) {
		return nil, nil
	}
	childPatchIds, err := r.sc.GetChildPatchIds(*v.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Couldn't find a patch with id: `%s` %s", *v.Id, err.Error()))
	}
	if len(childPatchIds) > 0 {
		childVersions := []*restModel.APIVersion{}
		for _, cp := range childPatchIds {
			// this calls the graphql Version query resolver
			cv, err := r.Query().Version(ctx, cp)
			if err != nil {
				//before erroring due to the version being nil or not found,
				// fetch the child patch to see if it's activated
				p, err := patch.FindOneId(cp)
				if err != nil {
					return nil, InternalServerError.Send(ctx, fmt.Sprintf("Encountered an error while fetching a child patch: %s", err.Error()))
				}
				if p == nil {
					return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to child patch %s", cp))
				}
				if p.Version != "" {
					//only return the error if the version is activated (and we therefore expect it to be there)
					return nil, InternalServerError.Send(ctx, "An unexpected error occurred. Could not find a child version and expected one.")
				}
			}
			if cv != nil {
				childVersions = append(childVersions, cv)
			}
		}
		return childVersions, nil
	}
	return nil, nil
}

func (r *versionResolver) TaskCount(ctx context.Context, obj *restModel.APIVersion) (*int, error) {
	taskCount, err := task.Count(task.ByVersion(*obj.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting task count for version `%s`: %s", *obj.Id, err.Error()))
	}
	return &taskCount, nil
}

func (r *versionResolver) VersionTiming(ctx context.Context, obj *restModel.APIVersion) (*VersionTiming, error) {
	v, err := model.VersionFindOneId(*obj.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding version `%s`: %s", *obj.Id, err.Error()))
	}
	if v == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding version `%s`: %s", *obj.Id, "Version not found"))
	}
	timeTaken, makespan, err := v.GetTimeSpent()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting timing for version `%s`: %s", *obj.Id, err.Error()))
	}
	// return nil if rounded timeTaken/makespan == 0s
	t := timeTaken.Round(time.Second)
	m := makespan.Round(time.Second)

	var apiTimeTaken restModel.APIDuration
	var apiMakespan restModel.APIDuration
	if t.Seconds() != 0 {
		apiTimeTaken = restModel.NewAPIDuration(t)
	}
	if m.Seconds() != 0 {
		apiMakespan = restModel.NewAPIDuration(m)
	}

	return &VersionTiming{
		TimeTaken: &apiTimeTaken,
		Makespan:  &apiMakespan,
	}, nil
}

func (r *versionResolver) BaseVersionID(ctx context.Context, obj *restModel.APIVersion) (*string, error) {
	baseVersion, err := model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(*obj.Project, *obj.Revision).WithFields(model.VersionIdentifierKey))
	if baseVersion == nil || err != nil {
		return nil, nil
	}
	return &baseVersion.Id, nil
}

func (r *versionResolver) Status(ctx context.Context, obj *restModel.APIVersion) (string, error) {
	failedAndAbortedStatuses := append(evergreen.TaskFailureStatuses, evergreen.TaskAborted)
	opts := data.TaskFilterOptions{
		Statuses:         failedAndAbortedStatuses,
		FieldsToProject:  []string{task.DisplayStatusKey},
		IncludeBaseTasks: false,
	}
	tasks, _, err := r.sc.FindTasksByVersion(*obj.Id, opts)
	if err != nil {
		return "", InternalServerError.Send(ctx, fmt.Sprintf("Could not fetch tasks for version: %s", err.Error()))
	}
	status, err := evergreen.VersionStatusToPatchStatus(*obj.Status)
	if err != nil {
		return "", InternalServerError.Send(ctx, fmt.Sprintf("An error occurred when converting a version status: %s", err.Error()))
	}
	if evergreen.IsPatchRequester(*obj.Requester) {
		p, err := r.sc.FindPatchById(*obj.Id)
		if err != nil {
			return status, InternalServerError.Send(ctx, fmt.Sprintf("Could not fetch Patch %s: %s", *obj.Id, err.Error()))
		}
		if len(p.ChildPatches) > 0 {
			patchStatuses := []string{*p.Status}
			for _, cp := range p.ChildPatches {
				patchStatuses = append(patchStatuses, *cp.Status)
				// add the child patch tasks to tasks so that we can consider their status
				childPatchTasks, _, err := r.sc.FindTasksByVersion(*cp.Id, opts)
				if err != nil {
					return "", InternalServerError.Send(ctx, fmt.Sprintf("Could not fetch tasks for patch: %s ", err.Error()))
				}
				tasks = append(tasks, childPatchTasks...)
			}
			status = patch.GetCollectiveStatus(patchStatuses)
		}
	}

	taskStatuses := getAllTaskStatuses(tasks)

	// If theres an aborted task we should set the patch status to aborted if there are no other failures
	if utility.StringSliceContains(taskStatuses, evergreen.TaskAborted) {
		if len(utility.StringSliceIntersection(taskStatuses, evergreen.TaskFailureStatuses)) == 0 {
			status = evergreen.PatchAborted
		}
	}
	return status, nil
}

func (r *Resolver) Version() VersionResolver { return &versionResolver{r} }

type ticketFieldsResolver struct{ *Resolver }

func (r *ticketFieldsResolver) AssigneeDisplayName(ctx context.Context, obj *thirdparty.TicketFields) (*string, error) {
	if obj.Assignee == nil {
		return nil, nil
	}
	return &obj.Assignee.DisplayName, nil
}

func (r *ticketFieldsResolver) AssignedTeam(ctx context.Context, obj *thirdparty.TicketFields) (*string, error) {
	if obj.AssignedTeam == nil {
		return nil, nil
	}
	if len(obj.AssignedTeam) != 0 {
		return &obj.AssignedTeam[0].Value, nil
	}
	return nil, nil
}

func (r *ticketFieldsResolver) JiraStatus(ctx context.Context, obj *thirdparty.TicketFields) (*string, error) {
	if obj.Status == nil {
		return nil, nil
	}
	return &obj.Status.Name, nil
}

func (r *ticketFieldsResolver) ResolutionName(ctx context.Context, obj *thirdparty.TicketFields) (*string, error) {
	if obj.Resolution == nil {
		return nil, nil
	}
	return &obj.Resolution.Name, nil
}

func (r *Resolver) TicketFields() TicketFieldsResolver { return &ticketFieldsResolver{r} }

func (r *taskResolver) Annotation(ctx context.Context, obj *restModel.APITask) (*restModel.APITaskAnnotation, error) {
	annotation, err := annotations.FindOneByTaskIdAndExecution(*obj.Id, obj.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding annotation: %s", err.Error()))
	}
	if annotation == nil {
		return nil, nil
	}
	apiAnnotation := restModel.APITaskAnnotationBuildFromService(*annotation)
	return apiAnnotation, nil
}

func (r *taskResolver) CanModifyAnnotation(ctx context.Context, obj *restModel.APITask) (bool, error) {
	authUser := gimlet.GetUser(ctx)
	permissions := gimlet.PermissionOpts{
		Resource:      *obj.ProjectId,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionAnnotations,
		RequiredLevel: evergreen.AnnotationsModify.Value,
	}
	if authUser.HasPermission(permissions) {
		return true, nil
	}
	if utility.StringSliceContains(evergreen.PatchRequesters, utility.FromStringPtr(obj.Requester)) {
		p, err := patch.FindOneId(utility.FromStringPtr(obj.Version))
		if err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding patch for task: %s", err.Error()))
		}
		if p == nil {
			return false, InternalServerError.Send(ctx, "patch for task doesn't exist")
		}
		if p.Author == authUser.Username() {
			return true, nil
		}
	}
	return false, nil
}

func (r *annotationResolver) WebhookConfigured(ctx context.Context, obj *restModel.APITaskAnnotation) (bool, error) {
	t, err := r.sc.FindTaskById(*obj.TaskId)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding task: %s", err.Error()))
	}
	if t == nil {
		return false, ResourceNotFound.Send(ctx, "error finding task for the task annotation")
	}
	_, ok, _ := plugin.IsWebhookConfigured(t.Project, t.Version)
	return ok, nil
}

func (r *issueLinkResolver) JiraTicket(ctx context.Context, obj *restModel.APIIssueLink) (*thirdparty.JiraTicket, error) {
	return restModel.GetJiraTicketFromURL(*obj.URL)

}

// New injects resources into the resolvers, such as the data connector
func New(apiURL string) Config {
	c := Config{
		Resolvers: &Resolver{
			sc: &data.DBConnector{URL: apiURL},
		},
	}
	c.Directives.RequireSuperUser = func(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
		user := gimlet.GetUser(ctx)
		if user == nil {
			return nil, Forbidden.Send(ctx, "user not logged in")
		}
		opts := gimlet.PermissionOpts{
			Resource:      evergreen.SuperUserPermissionsID,
			ResourceType:  evergreen.SuperUserResourceType,
			Permission:    evergreen.PermissionAdminSettings,
			RequiredLevel: evergreen.AdminSettingsEdit.Value,
		}
		if user.HasPermission(opts) {
			return next(ctx)
		}
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user %s does not have permission to access this resolver", user.Username()))
	}
	return c
}
