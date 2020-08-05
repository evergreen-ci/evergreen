package graphql

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"golang.org/x/crypto/ssh"
)

// GetGroupedFiles returns the files of a Task inside a GroupedFile struct
func GetGroupedFiles(ctx context.Context, name string, taskID string, execution int) (*GroupedFiles, error) {
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

func SetScheduled(ctx context.Context, sc data.Connector, taskID string, isActive bool) (*restModel.APITask, error) {
	usr := route.MustHaveUser(ctx)
	t, err := task.FindOneId(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if err = model.SetActiveState(t, usr.Username(), isActive); err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}

	// Get the modified task back out of the db
	t, err = task.FindOneId(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	apiTask := restModel.APITask{}
	err = apiTask.BuildFromService(t)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	err = apiTask.BuildFromService(sc.GetURL())
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return &apiTask, nil
}

// GetFormattedDate returns a time.Time type in the format "Dec 13, 2020, 11:58:04 pm"
func GetFormattedDate(t *time.Time, timezone string) (*string, error) {
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

// IsURL returns true if str is a url with scheme and domain name
func IsURL(str string) bool {
	u, err := url.ParseRequestURI(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func getVersionBaseTasks(d data.Connector, versionID string) ([]task.Task, error) {
	version, err := d.FindVersionById(versionID)
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

// BaseTaskStatuses represents the format {buildVariant: {displayName: status}} for base task statuses
type BaseTaskStatuses map[string]map[string]string

// GetBaseTaskStatusesFromPatchID gets the status of each base build associated with a task
func GetBaseTaskStatusesFromPatchID(d data.Connector, patchID string) (BaseTaskStatuses, error) {
	baseTasks, err := getVersionBaseTasks(d, patchID)
	if err != nil {
		return nil, err
	}
	baseTaskStatusesByDisplayNameByVariant := make(map[string]map[string]string)
	for _, task := range baseTasks {
		if _, ok := baseTaskStatusesByDisplayNameByVariant[task.BuildVariant]; !ok {
			baseTaskStatusesByDisplayNameByVariant[task.BuildVariant] = map[string]string{}
		}
		baseTaskStatusesByDisplayNameByVariant[task.BuildVariant][task.DisplayName] = task.GetDisplayStatus()
	}
	return baseTaskStatusesByDisplayNameByVariant, nil
}

// SchedulePatch schedules a patch. It returns an error and an HTTP status code. In the case of
// success, it also returns a success message and a version ID.
func SchedulePatch(ctx context.Context, patchId string, version *model.Version, patchUpdateReq PatchVariantsTasksRequest) (error, int, string, string) {
	var err error
	p, err := patch.FindOne(patch.ById(patch.NewId(patchId)))
	if err != nil {
		return errors.Errorf("error loading patch: %s", err), http.StatusInternalServerError, "", ""
	}

	// Unmarshal the project config and set it in the project context
	project := &model.Project{}
	if _, err = model.LoadProjectInto([]byte(p.PatchedConfig), p.Project, project); err != nil {
		return errors.Errorf("Error unmarshaling project config: %v", err), http.StatusInternalServerError, "", ""
	}

	grip.InfoWhen(len(patchUpdateReq.Tasks) > 0 || len(patchUpdateReq.Variants) > 0, message.Fields{
		"source":     "ui_update_patch",
		"message":    "legacy structure is being used",
		"update_req": patchUpdateReq,
		"patch_id":   p.Id.Hex(),
		"version":    p.Version,
	})

	tasks := model.TaskVariantPairs{}
	if len(patchUpdateReq.VariantsTasks) > 0 {
		tasks = model.VariantTasksToTVPairs(patchUpdateReq.VariantsTasks)
	} else {
		for _, v := range patchUpdateReq.Variants {
			for _, t := range patchUpdateReq.Tasks {
				if project.FindTaskForVariant(t, v) != nil {
					tasks.ExecTasks = append(tasks.ExecTasks, model.TVPair{Variant: v, TaskName: t})
				}
			}
		}
	}

	tasks.ExecTasks = model.IncludePatchDependencies(project, tasks.ExecTasks)

	if err = model.ValidateTVPairs(project, tasks.ExecTasks); err != nil {
		return err, http.StatusBadRequest, "", ""
	}

	// update the description for both reconfigured and new patches
	if err = p.SetDescription(patchUpdateReq.Description); err != nil {
		return errors.Wrap(err, "Error setting description"), http.StatusInternalServerError, "", ""
	}

	// update the description for both reconfigured and new patches
	if err = p.SetVariantsTasks(tasks.TVPairsToVariantTasks()); err != nil {
		return errors.Wrap(err, "Error setting description"), http.StatusInternalServerError, "", ""
	}

	if p.Version != "" {
		p.Activated = true
		// This patch has already been finalized, just add the new builds and tasks
		if version == nil {
			return errors.Errorf("Couldn't find patch for id %v", p.Version), http.StatusInternalServerError, "", ""
		}

		// First add new tasks to existing builds, if necessary
		err = model.AddNewTasksForPatch(context.Background(), p, version, project, tasks)
		if err != nil {
			return errors.Wrapf(err, "Error creating new tasks for version `%s`", version.Id), http.StatusInternalServerError, "", ""
		}

		err := model.AddNewBuildsForPatch(ctx, p, version, project, tasks)
		if err != nil {
			return errors.Wrapf(err, "Error creating new builds for version `%s`", version.Id), http.StatusInternalServerError, "", ""
		}

		return nil, http.StatusOK, "Builds and tasks successfully added to patch.", version.Id

	} else {
		env := evergreen.GetEnvironment()
		githubOauthToken, err := env.Settings().GetGithubOauthToken()
		if err != nil {
			return err, http.StatusBadRequest, "", ""
		}
		p.Activated = true
		err = p.SetVariantsTasks(tasks.TVPairsToVariantTasks())
		if err != nil {
			return errors.Wrap(err, "Error setting patch variants and tasks"), http.StatusInternalServerError, "", ""
		}

		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()

		requester := p.GetRequester()
		ver, err := model.FinalizePatch(ctx, p, requester, githubOauthToken)
		if err != nil {
			return errors.Wrap(err, "Error finalizing patch"), http.StatusInternalServerError, "", ""
		}
		if requester == evergreen.PatchVersionRequester {
			grip.Info(message.Fields{
				"operation":     "patch creation",
				"message":       "finalized patch",
				"from":          "UI",
				"patch_id":      p.Id,
				"variants":      p.BuildVariants,
				"tasks":         p.Tasks,
				"variant_tasks": p.VariantsTasks,
				"alias":         p.Alias,
			})
		}

		if p.IsGithubPRPatch() {
			job := units.NewGithubStatusUpdateJobForNewPatch(p.Id.Hex())
			if err := env.LocalQueue().Put(ctx, job); err != nil {
				return errors.Wrap(err, "Error adding github status update job to queue"), http.StatusInternalServerError, "", ""
			}
		}

		return nil, http.StatusOK, "Patch builds are scheduled.", ver.Id
	}
}

type VariantsAndTasksFromProject struct {
	Variants map[string]model.BuildVariant
	Tasks    []struct{ Name string }
	Project  model.Project
}

func GetVariantsAndTasksFromProject(patchedConfig string, patchProject string) (*VariantsAndTasksFromProject, error) {
	project := &model.Project{}
	if _, err := model.LoadProjectInto([]byte(patchedConfig), patchProject, project); err != nil {
		return nil, errors.Errorf("Error unmarshaling project config: %v", err)
	}

	// retrieve tasks and variant mappings' names
	variantMappings := make(map[string]model.BuildVariant)
	for _, variant := range project.BuildVariants {
		tasksForVariant := []model.BuildVariantTaskUnit{}
		for _, TaskFromVariant := range variant.Tasks {
			if TaskFromVariant.IsGroup {
				tasksForVariant = append(tasksForVariant, model.CreateTasksFromGroup(TaskFromVariant, project)...)
			} else {
				tasksForVariant = append(tasksForVariant, TaskFromVariant)
			}
		}
		variant.Tasks = tasksForVariant
		variantMappings[variant.Name] = variant
	}

	tasksList := []struct{ Name string }{}
	for _, task := range project.Tasks {
		// add a task name to the list if it's patchable and not restricted to git tags
		if !util.IsPtrSetToFalse(task.Patchable) && !util.IsPtrSetToTrue(task.GitTagOnly) {
			tasksList = append(tasksList, struct{ Name string }{task.Name})
		}
	}

	variantsAndTasksFromProject := VariantsAndTasksFromProject{
		Variants: variantMappings,
		Tasks:    tasksList,
		Project:  *project,
	}
	return &variantsAndTasksFromProject, nil
}

// GetPatchProjectVariantsAndTasksForUI gets the variants and tasks for a project for a patch id
func GetPatchProjectVariantsAndTasksForUI(ctx context.Context, apiPatch *restModel.APIPatch) (*PatchProject, error) {
	patchProjectVariantsAndTasks, err := GetVariantsAndTasksFromProject(*apiPatch.PatchedConfig, *apiPatch.Project)
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
		sort.SliceStable(projTasks, func(i, j int) bool {
			return projTasks[i] < projTasks[j]
		})
		projBuildVariant.Tasks = projTasks
		variants = append(variants, &projBuildVariant)
	}
	sort.SliceStable(variants, func(i, j int) bool {
		return variants[i].DisplayName < variants[j].DisplayName
	})
	// convert tasks to UI data structure
	tasks := []string{}
	for _, task := range patchProjectVariantsAndTasks.Tasks {
		tasks = append(tasks, task.Name)
	}

	patchProject := PatchProject{
		Variants: variants,
		Tasks:    tasks,
	}
	return &patchProject, nil
}

type PatchVariantsTasksRequest struct {
	VariantsTasks []patch.VariantTasks `json:"variants_tasks,omitempty"` // new format
	Variants      []string             `json:"variants"`                 // old format
	Tasks         []string             `json:"tasks"`                    // old format
	Description   string               `json:"description"`
}

// BuildFromGqlInput takes a PatchReconfigure gql type and returns a PatchVariantsTasksRequest type
func (p *PatchVariantsTasksRequest) BuildFromGqlInput(r PatchReconfigure) {
	p.Description = r.Description
	for _, vt := range r.VariantsTasks {
		variantTasks := patch.VariantTasks{
			Variant: vt.Variant,
			Tasks:   vt.Tasks,
		}
		for _, displayTask := range vt.DisplayTasks {
			dt := patch.DisplayTask{}
			dt.Name = displayTask.Name
			dt.ExecTasks = displayTask.ExecTasks
			variantTasks.DisplayTasks = append(variantTasks.DisplayTasks, dt)
		}
		p.VariantsTasks = append(p.VariantsTasks, variantTasks)
	}
}

// GetAPITaskFromTask builds an APITask from the given task
func GetAPITaskFromTask(ctx context.Context, sc data.Connector, task task.Task) (*restModel.APITask, error) {
	apiTask := restModel.APITask{}
	err := apiTask.BuildFromService(&task)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error building apiTask from task %s: %s", task.Id, err.Error()))
	}
	err = apiTask.BuildFromService(sc.GetURL())
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error setting building task from apiTask %s: %s", task.Id, err.Error()))
	}
	return &apiTask, nil
}

func FilterTasksByBaseStatuses(taskResults []*TaskResult, baseStatuses []string, baseTaskStatuses BaseTaskStatuses) []*TaskResult {
	tasksFilteredByBaseStatus := []*TaskResult{}
	for _, taskResult := range taskResults {
		if utility.StringSliceContains(baseStatuses, baseTaskStatuses[taskResult.BuildVariant][taskResult.DisplayName]) {
			tasksFilteredByBaseStatus = append(tasksFilteredByBaseStatus, taskResult)
		}
	}
	return tasksFilteredByBaseStatus
}
func ConvertDBTasksToGqlTasks(tasks []task.Task, baseTaskStatuses BaseTaskStatuses) []*TaskResult {
	var taskResults []*TaskResult
	for _, task := range tasks {
		t := TaskResult{
			ID:           task.Id,
			DisplayName:  task.DisplayName,
			Version:      task.Version,
			Status:       task.GetDisplayStatus(),
			BuildVariant: task.BuildVariant,
			BaseStatus:   baseTaskStatuses[task.BuildVariant][task.DisplayName],
		}
		taskResults = append(taskResults, &t)
	}
	return taskResults
}

type VersionModificationAction string

const (
	Restart     VersionModificationAction = "restart"
	SetActive   VersionModificationAction = "set_active"
	SetPriority VersionModificationAction = "set_priority"
)

type VersionModifications struct {
	Action   VersionModificationAction `json:"action"`
	Active   bool                      `json:"active"`
	Abort    bool                      `json:"abort"`
	Priority int64                     `json:"priority"`
	TaskIds  []string                  `json:"task_ids"`
}

func ModifyVersion(version model.Version, user user.DBUser, proj *model.ProjectRef, modifications VersionModifications) (int, error) {
	switch modifications.Action {
	case Restart:
		if err := model.RestartVersion(version.Id, modifications.TaskIds, modifications.Abort, user.Id); err != nil {
			return http.StatusInternalServerError, errors.Errorf("error restarting patch: %s", err)
		}
	case SetActive:
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
			if proj == nil {
				projRef, err := model.FindOneProjectRef(version.Identifier)
				if err != nil {
					return http.StatusNotFound, errors.Errorf("error getting project ref: %s", err)
				}
				if projRef == nil {
					return http.StatusNotFound, errors.Errorf("project for %s came back nil: %s", version.Branch, err)
				}
				proj = projRef
			}
			_, err := commitqueue.RemoveCommitQueueItemForVersion(proj.Identifier,
				proj.CommitQueue.PatchType, version.Id, user.DisplayName())
			if err != nil {
				return http.StatusInternalServerError, errors.Errorf("error removing patch from commit queue: %s", err)
			}
		}
	case SetPriority:
		if proj == nil {
			projRef, err := model.FindOneProjectRef(version.Identifier)
			if err != nil {
				return http.StatusNotFound, errors.Errorf("error getting project ref: %s", err)
			}
			if projRef == nil {
				return http.StatusNotFound, errors.Errorf("project for %s came back nil: %s", version.Branch, err)
			}
			proj = projRef
		}
		if modifications.Priority > evergreen.MaxTaskPriority {
			requiredPermission := gimlet.PermissionOpts{
				Resource:      proj.Identifier,
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

// ModifyVersionHandler handles the boilerplate code for performing a modify version action, i.e. schedule, unschedule, restart and set priority
func ModifyVersionHandler(ctx context.Context, dataConnector data.Connector, patchID string, modifications VersionModifications) error {
	version, err := dataConnector.FindVersionById(patchID)
	if err != nil {
		return ResourceNotFound.Send(ctx, fmt.Sprintf("error finding version %s: %s", patchID, err.Error()))
	}
	user := route.MustHaveUser(ctx)
	httpStatus, err := ModifyVersion(*version, *user, nil, modifications)
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

func isTaskBlocked(ctx context.Context, at *restModel.APITask) (*bool, error) {
	t, err := task.FindOneIdNewOrOld(*at.Id)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task %s not found", *at.Id))
	}
	isBlocked := t.Blocked()
	return &isBlocked, nil
}

func canRestartTask(ctx context.Context, at *restModel.APITask) (*bool, error) {
	taskBlocked, err := isTaskBlocked(ctx, at)
	if err != nil {
		return nil, err
	}
	nonrestartableStatuses := []string{evergreen.TaskStarted, evergreen.TaskUnstarted, evergreen.TaskUndispatched, evergreen.TaskDispatched, evergreen.TaskInactive}
	canRestart := !utility.StringSliceContains(nonrestartableStatuses, *at.Status) || at.Aborted || (at.DisplayOnly && *taskBlocked)
	return &canRestart, nil
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
	publicKeys := route.MustHaveUser(ctx).PublicKeys()
	for _, pubKey := range publicKeys {
		if pubKey.Name == publicKeyName {
			return true
		}
	}
	return false
}

func getMyPublicKeys(ctx context.Context) []*restModel.APIPubKey {
	usr := route.MustHaveUser(ctx)
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
