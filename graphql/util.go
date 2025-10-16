package graphql

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/api"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"golang.org/x/crypto/ssh"
)

// This file should consist only of private utility functions that are specific to graphql resolver use cases.

const (
	gitHashLength = 40 // A git hash contains 40 characters.
)

// getGroupedFiles returns the files of a Task inside a GroupedFile struct
func getGroupedFiles(ctx context.Context, name string, taskID string, execution int) (*GroupedFiles, error) {
	taskFiles, err := artifact.GetAllArtifacts(ctx, []artifact.TaskIDAndExecution{{TaskID: taskID, Execution: execution}})
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	hasUser := gimlet.GetUser(ctx) != nil
	strippedFiles, err := artifact.StripHiddenFiles(ctx, taskFiles, hasUser)
	if err != nil {
		return nil, err
	}

	env := evergreen.GetEnvironment()
	apiFileList := []*restModel.APIFile{}
	for _, file := range strippedFiles {
		apiFile := restModel.APIFile{}
		apiFile.BuildFromService(file)
		apiFile.GetLogURL(env, taskID, execution)
		apiFileList = append(apiFileList, &apiFile)
	}
	return &GroupedFiles{TaskName: &name, Files: apiFileList, TaskID: taskID, Execution: execution}, nil
}

func findAllTasksByIds(ctx context.Context, taskIDs ...string) ([]task.Task, error) {
	tasks, err := task.FindAll(ctx, db.Query(task.ByIds(taskIDs)))
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	if len(tasks) == 0 {
		return nil, ResourceNotFound.Send(ctx, "no tasks found")
	}
	if len(tasks) != len(taskIDs) {
		foundTaskIds := []string{}
		for _, ft := range tasks {
			foundTaskIds = append(foundTaskIds, ft.Id)
		}
		missingTaskIds, _ := utility.StringSliceSymmetricDifference(taskIDs, foundTaskIds)
		grip.Error(message.Fields{
			"message":       "could not find all tasks",
			"function":      "findAllTasksByIds",
			"missing_tasks": missingTaskIds,
		})
	}
	return tasks, nil
}

func setManyTasksScheduled(ctx context.Context, url string, isActive bool, taskIDs ...string) ([]*restModel.APITask, error) {
	usr := mustHaveUser(ctx)
	tasks, err := findAllTasksByIds(ctx, taskIDs...)
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if evergreen.IsGithubMergeQueueRequester(t.Requester) {
			return nil, InputValidationError.Send(ctx, "Merge queue tasks cannot be manually scheduled.")
		}
	}
	if err = model.SetActiveState(ctx, usr.Username(), isActive, tasks...); err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}

	// Get the modified tasks back out of the db
	tasks, err = findAllTasksByIds(ctx, taskIDs...)
	if err != nil {
		return nil, err
	}
	apiTasks := []*restModel.APITask{}
	for _, t := range tasks {
		apiTask := restModel.APITask{}
		err = apiTask.BuildFromService(ctx, &t, &restModel.APITaskArgs{
			LogURL: url,
		})
		if err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}

		apiTasks = append(apiTasks, &apiTask)
	}
	return apiTasks, nil
}

// getFormattedDate returns a time.Time type in the format "Dec 13, 2020, 11:58:04 pm"
func getFormattedDate(t *time.Time, timezone string) (*string, error) {
	if t == nil {
		return nil, nil
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, err
	}

	timeInUserTimezone := t.In(loc)
	newTime := fmt.Sprintf("%s %d, %d, %s", timeInUserTimezone.Month(), timeInUserTimezone.Day(), timeInUserTimezone.Year(), timeInUserTimezone.Format(time.Kitchen))

	return &newTime, nil
}

// GetDisplayStatus considers both child patch statuses and
// aborted status, and returns an overall status.
func getDisplayStatus(ctx context.Context, v *model.Version) (string, error) {
	status := v.Status
	if v.Aborted {
		status = evergreen.VersionAborted
	}
	if !evergreen.IsPatchRequester(v.Requester) || v.IsChild() {
		return status, nil
	}

	p, err := patch.FindOneId(ctx, v.Id)
	if err != nil {
		return "", errors.Wrapf(err, "finding patch '%s': %s", v.Id, err.Error())
	}
	if p == nil {
		return "", errors.Errorf("patch '%s' not found", v.Id)
	}
	allStatuses := []string{status}
	for _, cp := range p.Triggers.ChildPatches {
		cpVersion, err := model.VersionFindOneId(ctx, cp)
		if err != nil {
			return "", errors.Wrapf(err, "finding version for child patch '%s': %s", cp, err.Error())
		}
		if cpVersion == nil {
			continue
		}
		if cpVersion.Aborted {
			allStatuses = append(allStatuses, evergreen.VersionAborted)
		} else {
			allStatuses = append(allStatuses, cpVersion.Status)
		}
	}
	return patch.GetCollectiveStatusFromPatchStatuses(allStatuses), nil
}

// userCanModifyPatch checks if a user can make changes to a given patch. This is mainly to prevent
// users from modifying other users' patches.
func userCanModifyPatch(u *user.DBUser, patch patch.Patch) bool {
	if u == nil {
		return false
	}

	// Check if user is patch owner.
	if patch.Author == u.Username() {
		return true
	}

	// Check if user is superuser.
	permissions := gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionAdminSettings,
		RequiredLevel: evergreen.AdminSettingsEdit.Value,
	}
	if u.HasPermission(permissions) {
		return true
	}

	// Check if user is project admin.
	permissions = gimlet.PermissionOpts{
		Resource:      patch.Project,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsEdit.Value,
	}
	if u.HasPermission(permissions) {
		return true
	}

	// Check if user has patch admin permissions.
	permissions = gimlet.PermissionOpts{
		Resource:      patch.Project,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionPatches,
		RequiredLevel: evergreen.PatchSubmitAdmin.Value,
	}
	return u.HasPermission(permissions)
}

// getPatchProjectVariantsAndTasksForUI gets the variants and tasks for a project for a patch id
func getPatchProjectVariantsAndTasksForUI(ctx context.Context, apiPatch *restModel.APIPatch) (*PatchProject, error) {
	p, err := apiPatch.ToService()
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("converting APIPatch '%s' to service", utility.FromStringPtr(apiPatch.Id)))
	}
	patchProjectVariantsAndTasks, err := model.GetVariantsAndTasksFromPatchProject(ctx, evergreen.GetEnvironment().Settings(), &p)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting project variants and tasks for patch '%s': %s", utility.FromStringPtr(apiPatch.Id), err.Error()))
	}

	// convert variants to UI data structure
	variants := []*ProjectBuildVariant{}
	for _, buildVariant := range patchProjectVariantsAndTasks.Variants {
		projBuildVariant := ProjectBuildVariant{
			Name:        buildVariant.Name,
			DisplayName: buildVariant.DisplayName,
		}
		projTasks := []string{}
		executionTasks := map[string]bool{}
		for _, displayTask := range buildVariant.DisplayTasks {
			projTasks = append(projTasks, displayTask.Name)
			for _, execTask := range displayTask.ExecTasks {
				executionTasks[execTask] = true
			}
		}
		for _, taskUnit := range buildVariant.Tasks {
			// Only add task if it is not an execution task.
			if !executionTasks[taskUnit.Name] {
				projTasks = append(projTasks, taskUnit.Name)
			}
		}
		// Sort tasks alphanumerically by display name.
		sort.SliceStable(projTasks, func(i, j int) bool {
			return projTasks[i] < projTasks[j]
		})
		projBuildVariant.Tasks = projTasks
		variants = append(variants, &projBuildVariant)
	}
	sort.SliceStable(variants, func(i, j int) bool {
		return variants[i].DisplayName < variants[j].DisplayName
	})

	patchProject := PatchProject{
		Variants: variants,
	}
	return &patchProject, nil
}

// buildFromGqlInput takes a PatchConfigure gql type and returns a PatchUpdate type
func buildFromGqlInput(r PatchConfigure) model.PatchUpdate {
	p := model.PatchUpdate{}
	p.Description = r.Description
	p.PatchTriggerAliases = r.PatchTriggerAliases
	for i := range r.Parameters {
		p.Parameters = append(p.Parameters, r.Parameters[i].ToService())
	}
	for _, vt := range r.VariantsTasks {
		variantTasks := patch.VariantTasks{
			Variant: vt.Variant,
			Tasks:   vt.Tasks,
		}
		for _, displayTask := range vt.DisplayTasks {
			// note that the UI does not pass ExecTasks, which tells the back-end model figure out the right execution tasks
			dt := patch.DisplayTask{Name: displayTask.Name}
			variantTasks.DisplayTasks = append(variantTasks.DisplayTasks, dt)
		}
		p.VariantsTasks = append(p.VariantsTasks, variantTasks)
	}
	return p
}

// getAPITaskFromTask builds an APITask from the given task
func getAPITaskFromTask(ctx context.Context, url string, task task.Task) (*restModel.APITask, error) {
	apiTask := restModel.APITask{}
	err := apiTask.BuildFromService(ctx, &task, &restModel.APITaskArgs{
		LogURL: url,
	})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to APITask: %s", task.Id, err.Error()))
	}
	return &apiTask, nil
}

// getTask returns the task with the given id and execution number
func getTask(ctx context.Context, taskID string, execution *int, apiURL string) (*restModel.APITask, error) {
	dbTask, err := task.FindOneIdAndExecutionWithDisplayStatus(ctx, taskID, execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding task '%s': %s", taskID, err.Error()))
	}
	if dbTask == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	apiTask, err := getAPITaskFromTask(ctx, apiURL, *dbTask)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to APITask: %s", taskID, err.Error()))
	}
	return apiTask, err
}

// Takes a version id and some filter criteria and returns the matching associated tasks grouped together by their build variant.
func generateBuildVariants(ctx context.Context, versionId string, buildVariantOpts BuildVariantOptions, requester string, logURL string) ([]*GroupedBuildVariant, error) {
	var variantDisplayName = map[string]string{}
	var tasksByVariant = map[string][]*restModel.APITask{}
	defaultSort := []task.TasksSortOrder{
		{Key: task.DisplayNameKey, Order: 1},
	}
	baseVersionID := ""
	if buildVariantOpts.IncludeBaseTasks == nil {
		buildVariantOpts.IncludeBaseTasks = utility.ToBoolPtr(true)
	}
	if utility.FromBoolPtr(buildVariantOpts.IncludeBaseTasks) {
		baseVersion, err := model.FindBaseVersionForVersion(ctx, versionId)
		if err != nil {
			return nil, errors.Wrapf(err, "finding base version for version '%s'", versionId)
		}
		if baseVersion != nil {
			baseVersionID = baseVersion.Id
		}
	}
	includeNeverActivatedTasks := buildVariantOpts.IncludeNeverActivatedTasks
	if includeNeverActivatedTasks == nil {
		includeNeverActivatedTasks = utility.ToBoolPtr(false)
	}
	opts := task.GetTasksByVersionOptions{
		Statuses:      getValidTaskStatusesFilter(buildVariantOpts.Statuses),
		Variants:      buildVariantOpts.Variants,
		TaskNames:     buildVariantOpts.Tasks,
		Sorts:         defaultSort,
		BaseVersionID: baseVersionID,
		// Do not fetch inactive tasks for patches. This is because the UI does not display inactive tasks for patches.
		IncludeNeverActivatedTasks: *includeNeverActivatedTasks || !evergreen.IsPatchRequester(requester),
	}

	tasks, _, err := task.GetTasksByVersion(ctx, versionId, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "getting tasks for version '%s'", versionId)
	}

	for _, t := range tasks {
		apiTask := restModel.APITask{}
		err := apiTask.BuildFromService(ctx, &t, &restModel.APITaskArgs{
			LogURL: logURL,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "converting task '%s' to APITask: %s", t.Id, err.Error())
		}
		variantDisplayName[t.BuildVariant] = t.BuildVariantDisplayName
		tasksByVariant[t.BuildVariant] = append(tasksByVariant[t.BuildVariant], &apiTask)

	}

	result := []*GroupedBuildVariant{}
	for variant, tasks := range tasksByVariant {
		pbv := GroupedBuildVariant{
			Variant:     variant,
			DisplayName: variantDisplayName[variant],
			Tasks:       tasks,
		}
		result = append(result, &pbv)
	}

	// sort variants by name
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].DisplayName < result[j].DisplayName
	})

	return result, nil
}

// modifyVersionHandler handles the boilerplate code for performing a modify version action, i.e. schedule, unschedule, restart and set priority
func modifyVersionHandler(ctx context.Context, versionID string, modification model.VersionModification) error {
	v, err := model.VersionFindOneId(ctx, versionID)
	if err != nil {
		return ResourceNotFound.Send(ctx, fmt.Sprintf("finding version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}
	user := mustHaveUser(ctx)
	httpStatus, err := model.ModifyVersion(ctx, *v, *user, modification)
	if err != nil {
		return mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}
	return nil
}

func mapHTTPStatusToGqlError(ctx context.Context, httpStatus int, err error) *gqlerror.Error {
	switch httpStatus {
	case http.StatusInternalServerError:
		return InternalServerError.Send(ctx, err.Error())
	case http.StatusNotFound:
		return ResourceNotFound.Send(ctx, err.Error())
	case http.StatusUnauthorized:
		return Forbidden.Send(ctx, err.Error())
	case http.StatusBadRequest:
		return InputValidationError.Send(ctx, err.Error())
	default:
		return InternalServerError.Send(ctx, err.Error())
	}
}

func canRestartTask(ctx context.Context, t *task.Task) bool {
	// Cannot restart execution tasks.
	if t.IsPartOfDisplay(ctx) {
		return false
	}
	// It is possible to restart blocked display tasks. Later tasks in a display task could be blocked on
	// earlier tasks in the display task, in which case restarting the entire display task may unblock them.
	return (t.DisplayStatus == evergreen.TaskStatusBlocked && t.DisplayOnly) ||
		!utility.StringSliceContains(evergreen.TaskUncompletedStatuses, t.Status)
}

func canScheduleTask(ctx context.Context, t *task.Task) bool {
	// Cannot schedule execution tasks or aborted tasks.
	if t.IsPartOfDisplay(ctx) || t.Aborted {
		return false
	}
	if t.DisplayStatus != evergreen.TaskUnscheduled {
		return false
	}
	return true
}

func removeGeneralSubscriptions(usr *user.DBUser, subscriptions []event.Subscription) []string {
	filteredSubscriptions := make([]string, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		if !utility.StringSliceContains(usr.GeneralSubscriptionIDs(), subscription.ID) {
			filteredSubscriptions = append(filteredSubscriptions, subscription.ID)
		}
	}

	return filteredSubscriptions
}

func makePatchDuration(timeTaken, makeSpan string) *PatchDuration {
	res := &PatchDuration{}

	if timeTaken != "0s" {
		res.TimeTaken = &timeTaken
	}

	if makeSpan != "0s" {
		res.Makespan = &makeSpan
	}

	return res
}

func getResourceTypeAndIdFromSubscriptionSelectors(ctx context.Context, selectors []restModel.APISelector) (string, string, error) {
	var id string
	var idType string
	for _, s := range selectors {
		if s.Type == nil {
			return "", "", InputValidationError.Send(ctx, "Found nil for selector type. Selector type must be a string and not nil.")
		}
		// Don't exit the loop for object and id because together they
		// describe the resource id and resource type for the subscription
		switch *s.Type {
		case "object":
			idType = *s.Data
		case "id":
			id = *s.Data
		case "project":
			idType = "project"
			id = *s.Data
			return idType, id, nil
		case "in-version":
			idType = "version"
			id = *s.Data
			return idType, id, nil
		}
	}
	if idType == "" || id == "" {
		return "", "", InputValidationError.Send(ctx, "selectors do not include a target version, build, project, or task ID")
	}
	return idType, id, nil
}

func savePublicKey(ctx context.Context, publicKeyInput PublicKeyInput) error {
	if doesPublicKeyNameAlreadyExist(ctx, publicKeyInput.Name) {
		return InputValidationError.Send(ctx, fmt.Sprintf("provided key name '%s' already exists", publicKeyInput.Name))
	}
	err := verifyPublicKey(ctx, publicKeyInput)
	if err != nil {
		return err
	}
	err = mustHaveUser(ctx).AddPublicKey(ctx, publicKeyInput.Name, publicKeyInput.Key)
	if err != nil {
		return InternalServerError.Send(ctx, fmt.Sprintf("saving public key: %s", err.Error()))
	}
	return nil
}

func verifyPublicKey(ctx context.Context, publicKey PublicKeyInput) error {
	if publicKey.Name == "" {
		return InputValidationError.Send(ctx, "public key name cannot be empty")
	}
	_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey.Key))
	if err != nil {
		return InputValidationError.Send(ctx, fmt.Sprintf("provided public key is invalid: %s", err.Error()))
	}
	return nil
}

func doesPublicKeyNameAlreadyExist(ctx context.Context, publicKeyName string) bool {
	publicKeys := mustHaveUser(ctx).PublicKeys()
	for _, pubKey := range publicKeys {
		if pubKey.Name == publicKeyName {
			return true
		}
	}
	return false
}

func getMyPublicKeys(ctx context.Context) []*restModel.APIPubKey {
	usr := mustHaveUser(ctx)
	publicKeys := []*restModel.APIPubKey{}
	for _, item := range usr.PublicKeys() {
		currName := item.Name
		currKey := item.Key
		publicKeys = append(publicKeys, &restModel.APIPubKey{Name: &currName, Key: &currKey})
	}
	sort.SliceStable(publicKeys, func(i, j int) bool {
		return *publicKeys[i].Name < *publicKeys[j].Name
	})
	return publicKeys
}

func getAPIVolumeList(volumes []host.Volume) ([]*restModel.APIVolume, error) {
	apiVolumes := make([]*restModel.APIVolume, 0, len(volumes))
	for _, vol := range volumes {
		apiVolume := restModel.APIVolume{}
		apiVolume.BuildFromService(vol)
		apiVolumes = append(apiVolumes, &apiVolume)
	}
	return apiVolumes, nil
}

func mustHaveUser(ctx context.Context) *user.DBUser {
	u := gimlet.GetUser(ctx)
	if u == nil {
		grip.Error(message.Fields{
			"message": "no user attached to request expecting user",
		})
		return &user.DBUser{}
	}
	usr, valid := u.(*user.DBUser)
	if !valid {
		grip.Error(message.Fields{
			"message": "invalid user attached to request expecting user",
		})
		return &user.DBUser{}
	}

	return usr
}

func validateVolumeExpirationInput(ctx context.Context, expirationTime *time.Time, noExpiration *bool) error {
	if expirationTime != nil && noExpiration != nil && *noExpiration {
		return InputValidationError.Send(ctx, "cannot apply an expiration time AND set volume as non-expirable")
	}
	return nil
}

func validateVolumeName(ctx context.Context, name *string) error {
	if name == nil {
		return nil
	}
	if *name == "" {
		return InputValidationError.Send(ctx, "name cannot be empty")
	}
	usr := mustHaveUser(ctx)
	myVolumes, err := host.FindSortedVolumesByUser(ctx, usr.Id)
	if err != nil {
		return err
	}
	for _, vol := range myVolumes {
		if *name == vol.ID || *name == vol.DisplayName {
			return InputValidationError.Send(ctx, "provided volume name is already in use")
		}
	}
	return nil
}

func applyVolumeOptions(ctx context.Context, volume host.Volume, volumeOptions restModel.VolumeModifyOptions) error {
	// modify volume if volume options is not empty
	if volumeOptions != (restModel.VolumeModifyOptions{}) {
		mgr, err := cloud.GetEC2ManagerForVolume(ctx, &volume)
		if err != nil {
			return err
		}
		err = mgr.ModifyVolume(ctx, &volume, &volumeOptions)
		if err != nil {
			return InternalServerError.Send(ctx, fmt.Sprintf("applying expiration options to volume '%s': %s", volume.ID, err.Error()))
		}
	}
	return nil
}

func setVersionActivationStatus(ctx context.Context, version *model.Version) error {
	defaultSort := []task.TasksSortOrder{
		{Key: task.DisplayNameKey, Order: 1},
	}
	opts := task.GetTasksByVersionOptions{
		Sorts: defaultSort,
	}
	tasks, _, err := task.GetTasksByVersion(ctx, version.Id, opts)
	if err != nil {
		return errors.Wrapf(err, "getting tasks for version '%s'", version.Id)
	}
	return errors.Wrapf(version.SetActivated(ctx, task.AnyActiveTasks(tasks)), "updating version activated status for '%s'", version.Id)
}

func isPopulated(buildVariantOptions *BuildVariantOptions) bool {
	if buildVariantOptions == nil {
		return false
	}
	return len(buildVariantOptions.Tasks) > 0 || len(buildVariantOptions.Variants) > 0 || len(buildVariantOptions.Statuses) > 0
}

func getRedactedAPIVarsForProject(ctx context.Context, projectId string) (*restModel.APIProjectVars, error) {
	vars, err := model.FindOneProjectVars(ctx, projectId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding vars for project '%s': %s", projectId, err.Error()))
	}
	if vars == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("vars for project '%s' not found", projectId))
	}
	vars = vars.RedactPrivateVars()
	res := &restModel.APIProjectVars{}
	res.BuildFromService(*vars)
	return res, nil
}

func getAPIAliasesForProject(ctx context.Context, projectId string) ([]*restModel.APIProjectAlias, error) {
	aliases, err := model.FindAliasesForProjectFromDb(ctx, projectId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding aliases for project '%s': %s", projectId, err.Error()))
	}
	res := []*restModel.APIProjectAlias{}
	for _, alias := range aliases {
		apiAlias := restModel.APIProjectAlias{}
		apiAlias.BuildFromService(alias)
		res = append(res, &apiAlias)
	}
	return res, nil
}

func getAPISubscriptionsForOwner(ctx context.Context, ownerId string, ownerType event.OwnerType) ([]*restModel.APISubscription, error) {
	subscriptions, err := event.FindSubscriptionsByOwner(ctx, ownerId, ownerType)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding subscription for owner '%s' and type '%s': %s", ownerId, ownerType, err.Error()))
	}

	res := []*restModel.APISubscription{}
	for _, sub := range subscriptions {
		apiSubscription := restModel.APISubscription{}
		if err = apiSubscription.BuildFromService(sub); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting subscription '%s' to APISubscription: %s",
				sub.ID, err.Error()))
		}
		res = append(res, &apiSubscription)
	}
	return res, nil
}

func getPointerEventList(events []restModel.APIProjectEvent) []*restModel.APIProjectEvent {
	res := make([]*restModel.APIProjectEvent, len(events))
	for i := range events {
		res[i] = &events[i]
	}
	return res
}

// groupProjects takes a list of projects and groups them by their repo. If onlyDefaultedToRepo is true,
// it groups projects that defaulted to the repo under that repo and groups the rest under "".
func groupProjects(ctx context.Context, projects []model.ProjectRef, onlyDefaultedToRepo bool) ([]*GroupedProjects, error) {
	groupsMap := make(map[string][]*restModel.APIProjectRef)

	for _, p := range projects {
		// Do not include hidden projects in the final list of grouped projects, as they are considered
		// "deleted" projects.
		if p.IsHidden() {
			continue
		}

		groupName := fmt.Sprintf("%s/%s", p.Owner, p.Repo)
		if onlyDefaultedToRepo && !p.UseRepoSettings() {
			groupName = ""
		}

		apiProjectRef := restModel.APIProjectRef{}
		if err := apiProjectRef.BuildFromService(ctx, p); err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("converting project '%s' to APIProjectRef", p.Id))
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
			GroupDisplayName: groupName,
			Projects:         groupedProjects,
		}
		project := groupedProjects[0]
		if utility.FromBoolPtr(project.UseRepoSettings) {
			repoRefId := utility.FromStringPtr(project.RepoRefId)
			repoRef, err := model.FindOneRepoRef(ctx, repoRefId)
			if err != nil {
				return nil, err
			}

			if repoRef == nil {
				grip.Error(message.Fields{
					"message":     "repoRef not found",
					"repo_ref_id": repoRefId,
					"project":     project,
				})
			} else {
				apiRepoRef := restModel.APIProjectRef{}
				if err := apiRepoRef.BuildFromService(ctx, repoRef.ProjectRef); err != nil {
					return nil, errors.Wrap(err, fmt.Sprintf("converting repo '%s' to APIProjectRef", repoRef.ProjectRef.Id))
				}
				gp.Repo = &apiRepoRef
				if repoRef.ProjectRef.DisplayName != "" {
					gp.GroupDisplayName = repoRef.ProjectRef.DisplayName
				}
			}
		}

		groupsArr = append(groupsArr, &gp)
	}

	sort.SliceStable(groupsArr, func(i, j int) bool {
		return groupsArr[i].GroupDisplayName < groupsArr[j].GroupDisplayName
	})
	return groupsArr, nil
}

// getValidTaskStatusesFilter returns a slice of task statuses that are valid and are searchable.
// It returns an empty array if all is included as one of the entries
func getValidTaskStatusesFilter(statuses []string) []string {
	filteredStatuses := []string{}
	if utility.StringSliceContains(statuses, evergreen.TaskAll) {
		return filteredStatuses
	}
	filteredStatuses = utility.StringSliceIntersection(evergreen.TaskStatuses, statuses)
	return filteredStatuses
}

func bbGetCreatedTicketsPointers(ctx context.Context, taskId string) ([]*thirdparty.JiraTicket, error) {
	events, err := event.Find(ctx, event.TaskEventsForId(taskId))
	if err != nil {
		return nil, err
	}

	var results []*thirdparty.JiraTicket
	var searchTickets []string
	for _, evt := range events {
		data := evt.Data.(*event.TaskEventData)
		if evt.EventType == event.TaskJiraAlertCreated {
			searchTickets = append(searchTickets, data.JiraIssue)
		}
	}
	settings := evergreen.GetEnvironment().Settings()
	jiraHandler := thirdparty.NewJiraHandler(*settings.Jira.Export())
	for _, ticket := range searchTickets {
		jiraIssue, err := jiraHandler.GetJIRATicket(ticket)
		if err != nil {
			return nil, err
		}
		if jiraIssue == nil {
			continue
		}
		results = append(results, jiraIssue)
	}

	return results, nil
}

// getHostRequestOptions validates and transforms user-specified spawn host input
func getHostRequestOptions(ctx context.Context, usr *user.DBUser, spawnHostInput *SpawnHostInput) (*restModel.HostRequestOptions, error) {
	if spawnHostInput.SavePublicKey {
		if err := savePublicKey(ctx, *spawnHostInput.PublicKey); err != nil {
			return nil, err
		}
	}
	dist, err := distro.FindOneId(ctx, spawnHostInput.DistroID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding distro '%s': %s", spawnHostInput.DistroID, err.Error()))
	}
	if dist == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("distro '%s' not found", spawnHostInput.DistroID))
	}

	options := &restModel.HostRequestOptions{
		DistroID:             spawnHostInput.DistroID,
		Region:               spawnHostInput.Region,
		KeyName:              spawnHostInput.PublicKey.Key,
		IsVirtualWorkstation: spawnHostInput.IsVirtualWorkStation,
		NoExpiration:         spawnHostInput.NoExpiration,
	}
	if spawnHostInput.SleepSchedule != nil {
		options.SleepScheduleOptions = host.SleepScheduleOptions{
			WholeWeekdaysOff: spawnHostInput.SleepSchedule.WholeWeekdaysOff,
			DailyStartTime:   spawnHostInput.SleepSchedule.DailyStartTime,
			DailyStopTime:    spawnHostInput.SleepSchedule.DailyStopTime,
			TimeZone:         spawnHostInput.SleepSchedule.TimeZone,
		}
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
		if t, err = task.FindOneId(ctx, *spawnHostInput.TaskID); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding task '%s': %s", *spawnHostInput.TaskID, err.Error()))
		}
	}

	if utility.FromBoolPtr(spawnHostInput.UseProjectSetupScript) {
		if t == nil {
			return nil, InputValidationError.Send(ctx, "A valid task ID must be supplied when useProjectSetupScript is set to true.")
		}
		options.UseProjectSetupScript = *spawnHostInput.UseProjectSetupScript
	}

	if utility.FromBoolPtr(spawnHostInput.SpawnHostsStartedByTask) {
		if t == nil {
			return nil, InputValidationError.Send(ctx, "A valid task ID must be supplied when SpawnHostsStartedByTask is set to true.")
		}
		if err = data.CreateHostsFromTask(ctx, evergreen.GetEnvironment(), t, *usr, spawnHostInput.PublicKey.Key); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("spawning hosts from task '%s': %s", *spawnHostInput.TaskID, err.Error()))
		}
	}
	return options, nil
}

func getProjectMetadata(ctx context.Context, projectId *string, patchId *string) (*restModel.APIProjectRef, error) {
	projectRef, err := model.FindMergedProjectRef(ctx, *projectId, *patchId, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding merged project ref for project '%s': %s", utility.FromStringPtr(projectId), err.Error()))
	}
	if projectRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("merged project ref for project '%s' not found", utility.FromStringPtr(projectId)))
	}
	apiProjectRef := restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(ctx, *projectRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting project '%s' to APIProjectRef: %s", projectRef.Id, err.Error()))
	}
	return &apiProjectRef, nil
}

//////////////////////////////////
// Helper functions for task logs.
//////////////////////////////////

func getTaskLogs(ctx context.Context, obj *TaskLogs, logType task.TaskLogType) ([]*apimodels.LogMessage, error) {
	dbTask, err := task.FindOneIdAndExecution(ctx, obj.TaskID, obj.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding task '%s': %s", obj.TaskID, err.Error()))
	}
	if evergreen.IsUnstartedTaskStatus(dbTask.Status) {
		return []*apimodels.LogMessage{}, nil
	}

	it, err := dbTask.GetTaskLogs(ctx, task.TaskLogGetOptions{
		LogType: logType,
		TailN:   100,
	})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting logs for task '%s': %s", dbTask.Id, err.Error()))
	}

	lines, err := apimodels.ReadLogToSlice(it)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("reading logs for task '%s': %s", dbTask.Id, err.Error()))
	}

	return lines, nil
}

//////////////////////////////////////////
// Helper functions for task test results.
//////////////////////////////////////////

func convertTestFilterOptions(ctx context.Context, dbTask *task.Task, opts *TestFilterOptions) (*task.FilterOptions, error) {
	if opts == nil {
		return nil, nil
	}

	sort, baseTaskOpts, err := convertTestSortOptions(ctx, dbTask, opts.Sort)
	if err != nil {
		return nil, err
	}

	return &task.FilterOptions{
		TestName:            utility.FromStringPtr(opts.TestName),
		ExcludeDisplayNames: utility.FromBoolPtr(opts.ExcludeDisplayNames),
		Statuses:            opts.Statuses,
		GroupID:             utility.FromStringPtr(opts.GroupID),
		Sort:                sort,
		Limit:               utility.FromIntPtr(opts.Limit),
		Page:                utility.FromIntPtr(opts.Page),
		BaseTasks:           baseTaskOpts,
	}, nil
}

func convertTestSortOptions(ctx context.Context, dbTask *task.Task, opts []*TestSortOptions) ([]testresult.SortBy, []task.Task, error) {
	baseTaskOpts, err := getBaseTaskTestResultsOptions(ctx, dbTask)
	if err != nil {
		return nil, nil, err
	}

	var sort []testresult.SortBy
	for _, o := range opts {
		var key string
		switch o.SortBy {
		case TestSortCategoryStatus:
			key = testresult.SortByStatusKey
		case TestSortCategoryDuration:
			key = testresult.SortByDurationKey
		case TestSortCategoryTestName:
			key = testresult.SortByTestNameKey
		case TestSortCategoryStartTime:
			key = testresult.SortByStartKey
		case TestSortCategoryBaseStatus:
			if len(baseTaskOpts) == 0 {
				// Only sort by base status if we know there
				// are base task options we can send to the
				// results service.
				continue
			}
			key = testresult.SortByBaseStatusKey
		}

		sort = append(sort, testresult.SortBy{Key: key, OrderDSC: o.Direction == SortDirectionDesc})
	}

	return sort, baseTaskOpts, nil
}

func getBaseTaskTestResultsOptions(ctx context.Context, dbTask *task.Task) ([]task.Task, error) {
	var (
		baseTask *task.Task
		tasks    []task.Task
		err      error
	)

	if dbTask.Requester == evergreen.RepotrackerVersionRequester {
		baseTask, err = dbTask.FindTaskOnPreviousCommit(ctx)
	} else {
		baseTask, err = dbTask.FindTaskOnBaseCommit(ctx)
	}
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding base task for task '%s': %s", dbTask.Id, err.Error()))
	}

	if baseTask != nil {
		tasks, err = baseTask.GetTestResultsTasks(ctx)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("creating test results task options for base task '%s': %s", baseTask.Id, err.Error()))
		}
	}

	return tasks, nil
}

func handleDistroOnSaveOperation(ctx context.Context, distroID string, onSave DistroOnSaveOperation, userID string) (int, error) {
	noHostsUpdated := 0
	if onSave == DistroOnSaveOperationNone {
		return noHostsUpdated, nil
	}

	hosts, err := host.Find(ctx, host.ByDistroIDs(distroID))
	if err != nil {
		return noHostsUpdated, errors.Wrap(err, fmt.Sprintf("finding hosts for distro '%s'", distroID))
	}

	switch onSave {
	case DistroOnSaveOperationDecommission:
		if err = host.DecommissionHostsWithDistroId(ctx, distroID); err != nil {
			return noHostsUpdated, errors.Wrap(err, fmt.Sprintf("decommissioning hosts for distro '%s'", distroID))
		}
		for _, h := range hosts {
			event.LogHostStatusChanged(ctx, h.Id, h.Status, evergreen.HostDecommissioned, userID, "distro page")
		}
	case DistroOnSaveOperationReprovision:
		failed := []string{}
		for _, h := range hosts {
			if _, err = api.GetReprovisionToNewCallback(ctx, evergreen.GetEnvironment(), userID)(&h); err != nil {
				failed = append(failed, h.Id)
			}
		}
		if len(failed) > 0 {
			return len(hosts) - len(failed), errors.New(fmt.Sprintf("marking the following hosts for reprovision: %s", strings.Join(failed, ", ")))
		}
	case DistroOnSaveOperationRestartJasper:
		failed := []string{}
		for _, h := range hosts {
			if _, err = api.GetRestartJasperCallback(ctx, evergreen.GetEnvironment(), userID)(&h); err != nil {
				failed = append(failed, h.Id)
			}
		}
		if len(failed) > 0 {
			return len(hosts) - len(failed), errors.New(fmt.Sprintf("marking the following hosts for Jasper service restart: %s", strings.Join(failed, ", ")))
		}
	}

	return len(hosts), nil
}

func userHasDistroPermission(u *user.DBUser, distroId string, requiredLevel int) bool {
	opts := gimlet.PermissionOpts{
		Resource:      distroId,
		ResourceType:  evergreen.DistroResourceType,
		Permission:    evergreen.PermissionDistroSettings,
		RequiredLevel: requiredLevel,
	}
	return u.HasPermission(opts)
}

func userHasHostPermission(u *user.DBUser, distroId string, requiredLevel int, startedBy string) bool {
	opts := gimlet.PermissionOpts{
		Resource:      distroId,
		ResourceType:  evergreen.DistroResourceType,
		Permission:    evergreen.PermissionHosts,
		RequiredLevel: requiredLevel,
	}
	return u.Username() == startedBy || u.HasPermission(opts)
}

func userHasProjectSettingsPermission(u *user.DBUser, projectId string, requiredLevel int) bool {
	opts := gimlet.PermissionOpts{
		Resource:      projectId,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: requiredLevel,
	}
	return u.HasPermission(opts)
}

func makeDistroEvent(ctx context.Context, entry event.EventLogEntry) (*DistroEvent, error) {
	data, ok := entry.Data.(*event.DistroEventData)
	if !ok {
		return nil, errors.New("casting distro event data")
	}

	after, err := interfaceToMap(ctx, data.After)
	if err != nil {
		return nil, errors.Wrapf(err, "converting 'after' field to map")
	}

	before, err := interfaceToMap(ctx, data.Before)
	if err != nil {
		return nil, errors.Wrapf(err, "converting 'before' field to map")
	}

	legacyData, err := interfaceToMap(ctx, data.Data)
	if err != nil {
		return nil, errors.Wrapf(err, "converting legacy 'data' field to map")
	}

	user := data.User
	if user == "" {
		// Use legacy UserId field if User is undefined
		user = data.UserId
	}

	return &DistroEvent{
		After:     after,
		Before:    before,
		Data:      legacyData,
		Timestamp: entry.Timestamp,
		User:      user,
	}, nil
}

func makeAdminEvent(ctx context.Context, entry event.EventLogEntry) (*AdminEvent, error) {
	data, ok := entry.Data.(*event.AdminEventData)
	if !ok {
		return nil, errors.New("casting admin event data")
	}

	after, err := interfaceToMap(ctx, data.Changes.After)
	if err != nil {
		return nil, errors.Wrapf(err, "converting 'after' field to map")
	}

	before, err := interfaceToMap(ctx, data.Changes.Before)
	if err != nil {
		return nil, errors.Wrapf(err, "converting 'before' field to map")
	}

	return &AdminEvent{
		After:     after,
		Before:    before,
		Section:   utility.ToStringPtr(data.Section),
		Timestamp: entry.Timestamp,
		User:      data.User,
	}, nil
}

func interfaceToMap(ctx context.Context, data any) (map[string]any, error) {
	if data == nil {
		return nil, nil
	}

	mapField := map[string]any{}
	marshalledData, err := bson.Marshal(data)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling data")
	}

	if err = bson.Unmarshal(marshalledData, &mapField); err != nil {
		return nil, errors.Wrapf(err, "unmarshalling data")
	}

	return mapField, nil
}

func concurrentlyBuildVersionsMatchingTasksMap(ctx context.Context, versions []model.Version, opts task.HasMatchingTasksOptions) (map[string]bool, error) {
	wg := sync.WaitGroup{}
	input := make(chan model.Version, len(versions))
	output := make(chan string, len(versions))
	catcher := grip.NewBasicCatcher()
	hasMatchingTasksMap := map[string]bool{}

	// Populate the input channel that the goroutines will read from.
	for _, v := range versions {
		input <- v
	}
	close(input)

	// Limit number of parallel requests to the DB.
	const maxParallel = 20
	workers := util.Min(maxParallel, len(versions))
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range input {
				hasMatchingTasks, err := task.HasMatchingTasks(ctx, i.Id, opts)
				if err != nil {
					catcher.Add(err)
					continue
				}
				if hasMatchingTasks {
					output <- i.Id
				}
			}
		}()
	}
	wg.Wait()
	close(output)

	if catcher.HasErrors() {
		return nil, errors.Wrap(catcher.Resolve(), "finding matching tasks")
	}

	for versionId := range output {
		hasMatchingTasksMap[versionId] = true
	}

	return hasMatchingTasksMap, nil
}

func collapseCommit(ctx context.Context, mainlineCommits MainlineCommits, mainlineCommitVersion *MainlineCommitVersion, apiVersion restModel.APIVersion) {
	if len(mainlineCommits.Versions) > 0 {
		lastMainlineCommit := mainlineCommits.Versions[len(mainlineCommits.Versions)-1]
		if lastMainlineCommit.RolledUpVersions != nil {
			lastMainlineCommit.RolledUpVersions = append(lastMainlineCommit.RolledUpVersions, &apiVersion)
		} else {
			mainlineCommitVersion.RolledUpVersions = []*restModel.APIVersion{&apiVersion}
		}
	} else {
		mainlineCommitVersion.RolledUpVersions = []*restModel.APIVersion{&apiVersion}
	}
}

// getProjectPermissionLevel takes in ProjectPermission and AccessLevel (GraphQL-specific variables) and returns
// the equivalent Evergreen permission constants defined in globals.go.
func getProjectPermissionLevel(projectPermission ProjectPermission, access AccessLevel) (requiredPermission string, permissionInfo evergreen.PermissionLevel, err error) {
	var permission string
	var permissionLevel evergreen.PermissionLevel

	switch projectPermission {
	case ProjectPermissionSettings:
		permission = evergreen.PermissionProjectSettings
		if access == AccessLevelEdit {
			permissionLevel = evergreen.ProjectSettingsEdit
		} else if access == AccessLevelView {
			permissionLevel = evergreen.ProjectSettingsView
		} else {
			return "", permissionLevel, errors.Errorf("invalid access level for %s", evergreen.PermissionProjectSettings)
		}
	case ProjectPermissionPatches:
		permission = evergreen.PermissionPatches
		if access == AccessLevelAdmin {
			permissionLevel = evergreen.PatchSubmitAdmin
		} else if access == AccessLevelEdit {
			permissionLevel = evergreen.PatchSubmit
		} else {
			return "", permissionLevel, errors.Errorf("invalid access level for %s", evergreen.PermissionPatches)
		}
	case ProjectPermissionTasks:
		permission = evergreen.PermissionTasks
		if access == AccessLevelAdmin {
			permissionLevel = evergreen.TasksAdmin
		} else if access == AccessLevelEdit {
			permissionLevel = evergreen.TasksBasic
		} else if access == AccessLevelView {
			permissionLevel = evergreen.TasksView
		} else {
			return "", permissionLevel, errors.Errorf("invalid access level for %s", evergreen.PermissionTasks)
		}
	case ProjectPermissionAnnotations:
		permission = evergreen.PermissionAnnotations
		if access == AccessLevelEdit {
			permissionLevel = evergreen.AnnotationsModify
		} else if access == AccessLevelView {
			permissionLevel = evergreen.AnnotationsView
		} else {
			return "", permissionLevel, errors.Errorf("invalid access level for %s", evergreen.PermissionAnnotations)
		}
	case ProjectPermissionLogs:
		permission = evergreen.PermissionLogs
		if access == AccessLevelView {
			permissionLevel = evergreen.LogsView
		} else {
			return "", permissionLevel, errors.Errorf("invalid access level for %s", evergreen.PermissionLogs)
		}
	default:
		return "", permissionLevel, errors.New("invalid project permission")
	}

	return permission, permissionLevel, nil
}

func isPatchAuthorForTask(ctx context.Context, obj *restModel.APITask) (bool, error) {
	authUser := gimlet.GetUser(ctx)
	patchID := utility.FromStringPtr(obj.Version)
	if utility.StringSliceContains(evergreen.PatchRequesters, utility.FromStringPtr(obj.Requester)) {
		p, err := patch.FindOneId(ctx, patchID)
		if err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("finding patch '%s': %s", patchID, err.Error()))
		}
		if p == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("patch '%s' not found", patchID))
		}
		if p.Author == authUser.Username() {
			return true, nil
		}
	}
	return false, nil
}

func hasLogViewPermission(ctx context.Context, obj *restModel.APITask) bool {
	authUser := gimlet.GetUser(ctx)
	permissions := gimlet.PermissionOpts{
		Resource:      *obj.ProjectId,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionLogs,
		RequiredLevel: evergreen.LogsView.Value,
	}
	return authUser.HasPermission(permissions)
}

func hasAnnotationPermission(ctx context.Context, obj *restModel.APITask, requiredLevel int) (bool, error) {
	authUser := gimlet.GetUser(ctx)
	permissions := gimlet.PermissionOpts{
		Resource:      *obj.ProjectId,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionAnnotations,
		RequiredLevel: requiredLevel,
	}
	if authUser.HasPermission(permissions) {
		return true, nil
	}
	return isPatchAuthorForTask(ctx, obj)
}

func annotationPermissionHelper(ctx context.Context, taskID string, execution *int) error {
	t, err := getTask(ctx, taskID, execution, "")
	if err != nil {
		return err
	}
	canModify, err := hasAnnotationPermission(ctx, t, evergreen.AnnotationsModify.Value)
	if err != nil {
		return err
	}
	if !canModify {
		return Forbidden.Send(ctx, fmt.Sprintf("not authorized to modify annotation for task '%s'", taskID))
	}
	return nil
}

// flattenOtelVariables "flattens" one level of a string map. Any maps that are found as a value within the map are moved to the top level of the map, with "topkey.nestedkey" as their new key, in line with Honeycomb best practices.
func flattenOtelVariables(vars map[string]any) map[string]any {
	flattenedVars := map[string]any{}
	for k, v := range vars {
		if valueMap, isMap := v.(map[string]any); isMap {
			for nestedKey, nestedValue := range valueMap {
				flattenedVars[k+"."+nestedKey] = nestedValue
			}
		} else {
			flattenedVars[k] = v
		}
	}
	return flattenedVars
}

func setSingleTaskPriority(ctx context.Context, url string, taskID string, priority int) (*restModel.APITask, error) {
	t, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	authUser := mustHaveUser(ctx)
	if priority > evergreen.MaxTaskPriority {
		requiredPermission := gimlet.PermissionOpts{
			Resource:      t.Project,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionTasks,
			RequiredLevel: evergreen.TasksAdmin.Value,
		}
		isTaskAdmin := authUser.HasPermission(requiredPermission)
		if !isTaskAdmin {
			return nil, Forbidden.Send(ctx, fmt.Sprintf("not authorized to set priority %v, can only set priority less than or equal to %v", priority, evergreen.MaxTaskPriority))
		}
	}
	if err = model.SetTaskPriority(ctx, *t, int64(priority), authUser.Username()); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("setting task priority for '%s': %s", taskID, err.Error()))
	}

	t, err = task.FindOneId(ctx, taskID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	apiTask, err := getAPITaskFromTask(ctx, url, *t)
	return apiTask, err
}
