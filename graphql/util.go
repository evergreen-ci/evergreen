package graphql

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"golang.org/x/crypto/ssh"
)

// This file should consist only of private utility functions that are specific to graphql resolver use cases.

// getGroupedFiles returns the files of a Task inside a GroupedFile struct
func getGroupedFiles(ctx context.Context, name string, taskID string, execution int) (*GroupedFiles, error) {
	taskFiles, err := artifact.GetAllArtifacts([]artifact.TaskIDAndExecution{{TaskID: taskID, Execution: execution}})
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	hasUser := gimlet.GetUser(ctx) != nil
	strippedFiles, err := artifact.StripHiddenFiles(taskFiles, hasUser)
	if err != nil {
		return nil, err
	}

	apiFileList := []*restModel.APIFile{}
	for _, file := range strippedFiles {
		apiFile := restModel.APIFile{}
		err := apiFile.BuildFromService(file)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("error stripping hidden files"))
		}
		apiFileList = append(apiFileList, &apiFile)
	}
	return &GroupedFiles{TaskName: &name, Files: apiFileList}, nil
}

func findAllTasksByIds(ctx context.Context, taskIDs ...string) ([]task.Task, error) {
	tasks, err := task.FindAll(db.Query(task.ByIds(taskIDs)))
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if len(tasks) == 0 {
		return nil, ResourceNotFound.Send(ctx, errors.New("tasks not found").Error())
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
		if t.Requester == evergreen.MergeTestRequester && isActive {
			return nil, InputValidationError.Send(ctx, "commit queue tasks cannot be manually scheduled")
		}
	}
	if err = model.SetActiveState(usr.Username(), isActive, tasks...); err != nil {
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
		err = apiTask.BuildFromArgs(&t, &restModel.APITaskArgs{
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

func getVersionBaseTasks(versionID string) ([]task.Task, error) {
	version, err := model.VersionFindOneId(versionID)
	if err != nil {
		return nil, fmt.Errorf("Error getting version %s: %s", versionID, err.Error())
	}
	if version == nil {
		return nil, fmt.Errorf("No version found for ID %s", versionID)
	}
	baseVersion, err := model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(version.Identifier, version.Revision))
	if err != nil {
		return nil, fmt.Errorf("Error getting base version from version %s: %s", version.Id, err.Error())
	}
	if baseVersion == nil {
		return nil, fmt.Errorf("No base version found from version %s", version.Id)
	}
	baseTasks, err := task.FindTasksFromVersions([]string{baseVersion.Id})
	if err != nil {
		return nil, fmt.Errorf("Error getting tasks from version %s: %s", baseVersion.Id, err.Error())
	}
	if baseTasks == nil {
		return nil, fmt.Errorf("No tasks found for version %s", baseVersion.Id)
	}
	return baseTasks, nil
}

func hasEnqueuePatchPermission(u *user.DBUser, existingPatch *restModel.APIPatch) bool {
	if u == nil || existingPatch == nil {
		return false
	}

	// patch owner
	if utility.FromStringPtr(existingPatch.Author) == u.Username() {
		return true
	}

	// superuser
	permissions := gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionAdminSettings,
		RequiredLevel: evergreen.AdminSettingsEdit.Value,
	}
	if u.HasPermission(permissions) {
		return true
	}

	return u.HasPermission(gimlet.PermissionOpts{
		Resource:      utility.FromStringPtr(existingPatch.ProjectId),
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsEdit.Value,
	})
}

// getPatchProjectVariantsAndTasksForUI gets the variants and tasks for a project for a patch id
func getPatchProjectVariantsAndTasksForUI(ctx context.Context, apiPatch *restModel.APIPatch) (*PatchProject, error) {
	patchProjectVariantsAndTasks, err := model.GetVariantsAndTasksFromProject(ctx, *apiPatch.PatchedParserProject, *apiPatch.ProjectId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting project variants and tasks for patch %s: %s", *apiPatch.Id, err.Error()))
	}

	// convert variants to UI data structure
	variants := []*ProjectBuildVariant{}
	for _, buildVariant := range patchProjectVariantsAndTasks.Variants {
		projBuildVariant := ProjectBuildVariant{
			Name:        buildVariant.Name,
			DisplayName: buildVariant.DisplayName,
		}
		projTasks := []string{}
		for _, taskUnit := range buildVariant.Tasks {
			projTasks = append(projTasks, taskUnit.Name)
		}
		for _, displayTask := range buildVariant.DisplayTasks {
			projTasks = append(projTasks, displayTask.Name)
		}
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

// BuildFromGqlInput takes a PatchConfigure gql type and returns a PatchUpdate type
func buildFromGqlInput(r PatchConfigure) model.PatchUpdate {
	p := model.PatchUpdate{}
	p.Description = r.Description
	p.PatchTriggerAliases = r.PatchTriggerAliases
	for _, param := range r.Parameters {
		p.Parameters = append(p.Parameters, param.ToService())
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
	err := apiTask.BuildFromArgs(&task, &restModel.APITaskArgs{
		LogURL: url,
	})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error building apiTask from task %s: %s", task.Id, err.Error()))
	}
	return &apiTask, nil
}

// Takes a version id and some filter criteria and returns the matching associated tasks grouped together by their build variant.
func generateBuildVariants(versionId string, buildVariantOpts BuildVariantOptions) ([]*GroupedBuildVariant, error) {
	var variantDisplayName map[string]string = map[string]string{}
	var tasksByVariant map[string][]*restModel.APITask = map[string][]*restModel.APITask{}
	defaultSort := []task.TasksSortOrder{
		{Key: task.DisplayNameKey, Order: 1},
	}
	if buildVariantOpts.IncludeBaseTasks == nil {
		buildVariantOpts.IncludeBaseTasks = utility.ToBoolPtr(true)
	}
	opts := task.GetTasksByVersionOptions{
		Statuses:                       getValidTaskStatusesFilter(buildVariantOpts.Statuses),
		Variants:                       buildVariantOpts.Variants,
		TaskNames:                      buildVariantOpts.Tasks,
		Sorts:                          defaultSort,
		IncludeBaseTasks:               utility.FromBoolPtr(buildVariantOpts.IncludeBaseTasks),
		IncludeBuildVariantDisplayName: true,
	}
	start := time.Now()
	tasks, _, err := task.GetTasksByVersion(versionId, opts)
	if err != nil {
		return nil, errors.Wrapf(err, fmt.Sprintf("Error getting tasks for patch `%s`", versionId))
	}
	timeToFindTasks := time.Since(start)
	buildTaskStartTime := time.Now()
	for _, t := range tasks {
		apiTask := restModel.APITask{}
		err := apiTask.BuildFromArgs(&t, nil)
		if err != nil {
			return nil, errors.Wrapf(err, fmt.Sprintf("Error building apiTask from task : %s", t.Id))
		}
		variantDisplayName[t.BuildVariant] = t.BuildVariantDisplayName
		tasksByVariant[t.BuildVariant] = append(tasksByVariant[t.BuildVariant], &apiTask)

	}

	timeToBuildTasks := time.Since(buildTaskStartTime)
	groupTasksStartTime := time.Now()

	result := []*GroupedBuildVariant{}
	for variant, tasks := range tasksByVariant {
		pbv := GroupedBuildVariant{
			Variant:     variant,
			DisplayName: variantDisplayName[variant],
			Tasks:       tasks,
		}
		result = append(result, &pbv)
	}

	timeToGroupTasks := time.Since(groupTasksStartTime)

	sortTasksStartTime := time.Now()
	// sort variants by name
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].DisplayName < result[j].DisplayName
	})

	timeToSortTasks := time.Since(sortTasksStartTime)

	totalTime := time.Since(start)
	grip.InfoWhen(totalTime > time.Second*2, message.Fields{
		"Ticket":             "EVG-14828",
		"timeToFindTasksMS":  timeToFindTasks.Milliseconds(),
		"timeToBuildTasksMS": timeToBuildTasks.Milliseconds(),
		"timeToGroupTasksMS": timeToGroupTasks.Milliseconds(),
		"timeToSortTasksMS":  timeToSortTasks.Milliseconds(),
		"totalTimeMS":        totalTime.Milliseconds(),
		"versionId":          versionId,
		"taskCount":          len(tasks),
		"buildVariantCount":  len(result),
	})
	return result, nil
}

// getFailedTestResultsSample returns a sample of failed test results for the given tasks that match the supplied testFilters
func getCedarFailedTestResultsSample(ctx context.Context, tasks []task.Task, testFilters []string) ([]apimodels.CedarFailedTestResultsSample, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	taskFilters := []apimodels.CedarTaskInfo{}
	for _, t := range tasks {
		taskFilters = append(taskFilters, apimodels.CedarTaskInfo{
			TaskID:      t.Id,
			Execution:   t.Execution,
			DisplayTask: t.DisplayOnly,
		})
	}

	opts := apimodels.GetCedarFailedTestResultsSampleOptions{
		BaseURL: evergreen.GetEnvironment().Settings().Cedar.BaseURL,
		SampleOptions: apimodels.CedarFailedTestSampleOptions{
			Tasks:        taskFilters,
			RegexFilters: testFilters,
		},
	}
	results, err := apimodels.GetCedarFilteredFailedSamples(ctx, opts)
	if err != nil {
		return nil, errors.Wrap(err, "getting cedar filtered failed samples")
	}
	return results, nil
}

// modifyVersionHandler handles the boilerplate code for performing a modify version action, i.e. schedule, unschedule, restart and set priority
func modifyVersionHandler(ctx context.Context, patchID string, modification model.VersionModification) error {
	v, err := model.VersionFindOneId(patchID)
	if err != nil {
		return ResourceNotFound.Send(ctx, fmt.Sprintf("error finding version %s: %s", patchID, err.Error()))
	}
	if v == nil {
		return ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find version with id: `%s`", patchID))
	}
	user := mustHaveUser(ctx)
	httpStatus, err := model.ModifyVersion(*v, *user, modification)
	if err != nil {
		return mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	if evergreen.IsPatchRequester(v.Requester) {
		// restart is handled through graphql because we need the user to specify
		// which downstream tasks they want to restart
		if modification.Action != evergreen.RestartAction {
			//do the same for child patches
			p, err := patch.FindOneId(patchID)
			if err != nil {
				return ResourceNotFound.Send(ctx, fmt.Sprintf("error finding patch %s: %s", patchID, err.Error()))
			}
			if p == nil {
				return ResourceNotFound.Send(ctx, fmt.Sprintf("patch '%s' not found ", patchID))
			}
			if p.IsParent() {
				childPatches, err := patch.Find(patch.ByStringIds(p.Triggers.ChildPatches))
				if err != nil {
					return InternalServerError.Send(ctx, fmt.Sprintf("error getting child patches: %s", err.Error()))
				}
				for _, childPatch := range childPatches {
					// only modify the child patch if it is finalized
					if childPatch.Version != "" {
						err = modifyVersionHandler(ctx, childPatch.Id.Hex(), modification)
						if err != nil {
							return errors.Wrap(mapHTTPStatusToGqlError(ctx, httpStatus, err), fmt.Sprintf("error modifying child patch '%s'", patchID))
						}
					}

				}
			}

		}
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

func isTaskBlocked(ctx context.Context, at *restModel.APITask) (bool, error) {
	i, err := at.ToService()
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to service", *at.Id))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("converting APITask '%s' to Task", *at.Id))
	}
	isBlocked := t.Blocked()
	return isBlocked, nil
}

func isExecutionTask(ctx context.Context, at *restModel.APITask) (bool, error) {
	i, err := at.ToService()
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to service", *at.Id))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("converting APITask '%s' to Task", *at.Id))
	}
	isExecutionTask := t.IsPartOfDisplay()
	return isExecutionTask, nil
}

func canRestartTask(ctx context.Context, at *restModel.APITask) (bool, error) {
	taskBlocked, err := isTaskBlocked(ctx, at)
	if err != nil {
		return false, err
	}
	canRestart := !utility.StringSliceContains(evergreen.TaskUncompletedStatuses, *at.Status) || at.Aborted || (at.DisplayOnly && taskBlocked)
	isExecTask, err := isExecutionTask(ctx, at) // Cant restart execution tasks.
	if err != nil {
		return false, err
	}
	if isExecTask {
		canRestart = false
	}
	return canRestart, nil
}

func getAllTaskStatuses(tasks []task.Task) []string {
	statusesMap := map[string]bool{}
	for _, task := range tasks {
		statusesMap[task.GetDisplayStatus()] = true
	}
	statusesArr := []string{}
	for key := range statusesMap {
		statusesArr = append(statusesArr, key)
	}
	sort.SliceStable(statusesArr, func(i, j int) bool {
		return statusesArr[i] < statusesArr[j]
	})
	return statusesArr
}

func formatDuration(duration string) string {
	regex := regexp.MustCompile(`\d*[dhms]`)
	return strings.TrimSpace(regex.ReplaceAllStringFunc(duration, func(m string) string {
		return m + " "
	}))
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
		return "", "", InputValidationError.Send(ctx, "Selectors do not indicate a target version, build, project, or task ID")
	}
	return idType, id, nil
}

func savePublicKey(ctx context.Context, publicKeyInput PublicKeyInput) error {
	if doesPublicKeyNameAlreadyExist(ctx, publicKeyInput.Name) {
		return InputValidationError.Send(ctx, fmt.Sprintf("Provided key name, %s, already exists.", publicKeyInput.Name))
	}
	err := verifyPublicKey(ctx, publicKeyInput)
	if err != nil {
		return err
	}
	err = mustHaveUser(ctx).AddPublicKey(publicKeyInput.Name, publicKeyInput.Key)
	if err != nil {
		return InternalServerError.Send(ctx, fmt.Sprintf("Error saving public key: %s", err.Error()))
	}
	return nil
}

func verifyPublicKey(ctx context.Context, publicKey PublicKeyInput) error {
	if publicKey.Name == "" {
		return InputValidationError.Send(ctx, fmt.Sprintf("Provided public key name cannot be empty."))
	}
	_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey.Key))
	if err != nil {
		return InputValidationError.Send(ctx, fmt.Sprintf("Provided public key is invalid : %s", err.Error()))
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
		if err := apiVolume.BuildFromService(vol); err != nil {
			return nil, errors.Wrapf(err, "error building volume '%s' from service", vol.ID)
		}
		apiVolumes = append(apiVolumes, &apiVolume)
	}
	return apiVolumes, nil
}

func SpawnHostForTestCode(ctx context.Context, vol *host.Volume, h *host.Host) error {
	mgr, err := cloud.GetEC2ManagerForVolume(ctx, vol)
	if err != nil {
		return err
	}
	if os.Getenv("SETTINGS_OVERRIDE") != "" {
		// The mock manager needs to spawn the host specified in our test data.
		// The host should already be spawned in a non-test scenario.
		_, err := mgr.SpawnHost(ctx, h)
		if err != nil {
			return errors.Wrapf(err, "error spawning host in test code")
		}
	}
	return nil
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
	if expirationTime != nil && noExpiration != nil && *noExpiration == true {
		return InputValidationError.Send(ctx, "Cannot apply an expiration time AND set volume as non-expirable")
	}
	return nil
}

func validateVolumeName(ctx context.Context, name *string) error {
	if name == nil {
		return nil
	}
	if *name == "" {
		return InputValidationError.Send(ctx, "Name cannot be empty.")
	}
	usr := mustHaveUser(ctx)
	myVolumes, err := host.FindSortedVolumesByUser(usr.Id)
	if err != nil {
		return err
	}
	for _, vol := range myVolumes {
		if *name == vol.ID || *name == vol.DisplayName {
			return InputValidationError.Send(ctx, "The provided volume name is already in use")
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
			return InternalServerError.Send(ctx, fmt.Sprintf("Unable to apply expiration options to volume %s: %s", volume.ID, err.Error()))
		}
	}
	return nil
}

func setVersionActivationStatus(version *model.Version) error {
	defaultSort := []task.TasksSortOrder{
		{Key: task.DisplayNameKey, Order: 1},
	}
	opts := task.GetTasksByVersionOptions{
		Sorts:                          defaultSort,
		IncludeBaseTasks:               false,
		IncludeBuildVariantDisplayName: false,
	}
	tasks, _, err := task.GetTasksByVersion(version.Id, opts)
	if err != nil {
		return errors.Wrapf(err, "error getting tasks for version %s", version.Id)
	}
	if !task.AnyActiveTasks(tasks) {
		return errors.Wrapf(version.SetNotActivated(), "Error updating version activated status for `%s`", version.Id)
	} else {
		return errors.Wrapf(version.SetActivated(), "Error updating version activated status for `%s`", version.Id)
	}
}
func (buildVariantOptions *BuildVariantOptions) isPopulated() bool {
	if buildVariantOptions == nil {
		return false
	}
	return len(buildVariantOptions.Tasks) > 0 || len(buildVariantOptions.Variants) > 0 || len(buildVariantOptions.Statuses) > 0
}

func getRedactedAPIVarsForProject(ctx context.Context, projectId string) (*restModel.APIProjectVars, error) {
	vars, err := model.FindOneProjectVars(projectId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding project vars for '%s': %s", projectId, err.Error()))
	}
	if vars == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("vars for '%s' don't exist", projectId))
	}
	vars = vars.RedactPrivateVars()
	res := &restModel.APIProjectVars{}
	if err = res.BuildFromService(vars); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem building APIProjectVars from service: %s", err.Error()))
	}
	return res, nil
}

func getAPIAliasesForProject(ctx context.Context, projectId string) ([]*restModel.APIProjectAlias, error) {
	aliases, err := model.FindAliasesForProjectFromDb(projectId)
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

func getAPISubscriptionsForProject(ctx context.Context, projectId string) ([]*restModel.APISubscription, error) {
	subscriptions, err := event.FindSubscriptionsByOwner(projectId, event.OwnerTypeProject)
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

func getPointerEventList(events []restModel.APIProjectEvent) []*restModel.APIProjectEvent {
	res := make([]*restModel.APIProjectEvent, len(events))
	for i := range events {
		res[i] = &events[i]
	}
	return res
}

// groupProjects takes a list of projects and groups them by their repo. If onlyDefaultedToRepo is true,
// it groups projects that defaulted to the repo under that repo and groups the rest under "".
func groupProjects(projects []model.ProjectRef, onlyDefaultedToRepo bool) ([]*GroupedProjects, error) {
	groupsMap := make(map[string][]*restModel.APIProjectRef)

	for _, p := range projects {
		groupName := fmt.Sprintf("%s/%s", p.Owner, p.Repo)
		if onlyDefaultedToRepo && !p.UseRepoSettings() {
			groupName = ""
		}

		apiProjectRef := restModel.APIProjectRef{}
		if err := apiProjectRef.BuildFromService(p); err != nil {
			return nil, errors.Wrap(err, "error building APIProjectRef from service")
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
			Name:             groupName, //deprecated
			GroupDisplayName: groupName,
			Projects:         groupedProjects,
		}
		project := groupedProjects[0]
		if utility.FromBoolPtr(project.UseRepoSettings) {
			repoRefId := utility.FromStringPtr(project.RepoRefId)
			repoRef, err := model.FindOneRepoRef(repoRefId)
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
				if err := apiRepoRef.BuildFromService(repoRef.ProjectRef); err != nil {
					return nil, errors.Wrap(err, "error building the repo's ProjectRef from service")
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

func hasProjectPermission(ctx context.Context, resource string, next graphql.Resolver, permissionLevel int) (res interface{}, err error) {
	user := gimlet.GetUser(ctx)
	if user == nil {
		return nil, Forbidden.Send(ctx, "user not logged in")
	}
	opts := gimlet.PermissionOpts{
		Resource:      resource,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: permissionLevel,
	}
	if user.HasPermission(opts) {
		return next(ctx)
	}
	return nil, Forbidden.Send(ctx, fmt.Sprintf("user %s does not have permission to access settings for the project %s", user.Username(), resource))
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

func getCollectiveStatusArray(v restModel.APIVersion) ([]string, error) {
	status, err := evergreen.VersionStatusToPatchStatus(*v.Status)
	if err != nil {
		return nil, errors.Wrap(err, "converting a version status")
	}
	isAborted := utility.FromBoolPtr(v.Aborted)
	allStatuses := []string{}
	if isAborted {
		allStatuses = append(allStatuses, evergreen.PatchAborted)

	} else {
		allStatuses = append(allStatuses, status)
	}
	if evergreen.IsPatchRequester(*v.Requester) {
		p, err := data.FindPatchById(*v.Id)
		if err != nil {
			return nil, errors.Wrapf(err, "fetching patch '%s'", *v.Id)
		}
		if len(p.ChildPatches) > 0 {
			for _, cp := range p.ChildPatches {
				cpVersion, err := model.VersionFindOneId(*cp.Version)
				if err != nil {
					return nil, errors.Wrapf(err, "fetching version for patch '%s'", *v.Id)
				}
				if cpVersion == nil {
					continue
				}
				if cpVersion.Aborted {
					allStatuses = append(allStatuses, evergreen.PatchAborted)
				} else {
					allStatuses = append(allStatuses, *cp.Status)
				}
			}
		}
	}
	return allStatuses, nil
}
