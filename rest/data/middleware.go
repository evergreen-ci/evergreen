package data

import (
	"context"
	"net/http"
	"net/url"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

const (
	projectIdKey  = "projectId"
	identifierKey = "identifier"

	// Keys that are used in the paramsMap.
	projectIdentifierKey = "projectIdentifier"
	repoIdKey            = "repoId"
	versionIdKey         = "versionId"
	patchIdKey           = "patchId"
	buildIdKey           = "buildId"
	logIdKey             = "logId"
	taskIdKey            = "taskId"
)

// GetProjectIdFromParams queries for a project and returns its projectId as a string.
// It performs different queries based on the keys that exist in the paramsMap input.
// If an error occurs, it will return an empty string, a status code, and the error itself.
func GetProjectIdFromParams(ctx context.Context, paramsMap map[string]string) (string, int, error) {
	projectID := util.CoalesceString(paramsMap[projectIdKey], paramsMap[projectIdentifierKey])
	repoID := paramsMap[repoIdKey]

	var err error
	versionID := paramsMap[versionIdKey]
	if projectID == "" && versionID != "" {
		projectID, err = model.FindProjectForVersion(ctx, versionID)
		if err != nil {
			return "", http.StatusNotFound, errors.Wrapf(err, "finding version '%s'", versionID)
		}
	}

	patchID := paramsMap[patchIdKey]
	if projectID == "" && patchID != "" {
		if !patch.IsValidId(patchID) {
			return "", http.StatusBadRequest, errors.New("not a valid patch ID")
		}
		projectID, err = patch.FindProjectForPatch(ctx, patch.NewId(patchID))
		if err != nil {
			return "", http.StatusNotFound, errors.Wrapf(err, "finding project for patch '%s'", patchID)
		}
	}

	buildID := paramsMap[buildIdKey]
	if projectID == "" && buildID != "" {
		projectID, err = build.FindProjectForBuild(ctx, buildID)
		if err != nil {
			return "", http.StatusNotFound, errors.Wrapf(err, "finding project for build '%s'", buildID)
		}
	}

	testLog := paramsMap[logIdKey]
	if projectID == "" && testLog != "" {
		var test *testlog.TestLog
		test, err = testlog.FindOneTestLogById(ctx, testLog)
		if err != nil {
			return "", http.StatusInternalServerError, errors.Wrapf(err, "finding test log '%s'", testLog)
		}
		if test == nil {
			return "", http.StatusNotFound, errors.Errorf("test log '%s' not found", testLog)
		}
		projectID, err = task.FindProjectForTask(ctx, test.Task)
		if err != nil {
			return "", http.StatusNotFound, errors.Wrapf(err, "finding project for task '%s' associated with test log '%s'", test.Task, test.Id)
		}
	}

	taskID := paramsMap[taskIdKey]
	if projectID == "" && taskID != "" {
		projectID, err = task.FindProjectForTask(ctx, taskID)
		if err != nil {
			return "", http.StatusNotFound, errors.Wrapf(err, "finding project for task '%s'", taskID)
		}
	}

	if repoID != "" {
		var repoRef *model.RepoRef
		repoRef, err = model.FindOneRepoRef(ctx, repoID)
		if err != nil {
			return "", http.StatusInternalServerError, errors.Wrap(err, "finding repo")
		}
		if repoRef == nil {
			return "", http.StatusNotFound, errors.Errorf("repo '%s' not found", repoID)
		}
		return repoID, http.StatusOK, nil
	}

	// Return an error if the project isn't found
	if projectID == "" {
		return "", http.StatusNotFound, errors.New("no project found")
	}

	projectRef, err := model.FindMergedProjectRef(ctx, projectID, versionID, true)
	if err != nil {
		return "", http.StatusInternalServerError, errors.Wrap(err, "finding project")
	}
	var id string
	if projectRef != nil {
		id = projectRef.Id
	} else {
		// If the project wasn't found, it's sometimes because it's a repo
		// ref id.
		repoRef, err := model.FindOneRepoRef(ctx, projectID)
		if err != nil {
			return "", http.StatusInternalServerError, errors.Wrap(err, "finding repo project")
		}
		if repoRef == nil {
			return "", http.StatusNotFound, errors.Errorf("project/repo '%s' not found", projectID)
		}
		id = repoRef.Id
	}

	usr := gimlet.GetUser(ctx)
	if usr == nil {
		return "", http.StatusUnauthorized, errors.New("unauthorized")
	}

	if id == "" {
		return "", http.StatusInternalServerError, errors.New("project ID is blank")
	}

	return id, http.StatusOK, nil
}

// BuildProjectParameterMapForGraphQL builds the parameters map that can be used as an input to GetProjectIdFromParams.
// It is used by the GraphQL @requireProjectAccess directive.
func BuildProjectParameterMapForGraphQL(args map[string]any) (map[string]string, error) {
	paramsMap := map[string]string{}

	if projectIdentifier, hasProjectIdentifier := args[projectIdentifierKey].(string); hasProjectIdentifier {
		paramsMap[projectIdentifierKey] = projectIdentifier
	}
	if identifier, hasIdentifier := args[identifierKey].(string); hasIdentifier {
		paramsMap[projectIdentifierKey] = identifier
	}
	if projectId, hasProjectId := args[projectIdKey].(string); hasProjectId {
		paramsMap[projectIdentifierKey] = projectId
	}
	if repoId, hasRepoId := args[repoIdKey].(string); hasRepoId {
		paramsMap[repoIdKey] = repoId
	}
	if versionId, hasVersionId := args[versionIdKey].(string); hasVersionId {
		paramsMap[versionIdKey] = versionId
	}
	if patchId, hasPatchId := args[patchIdKey].(string); hasPatchId {
		paramsMap[patchIdKey] = patchId
	}
	if taskId, hasTaskId := args[taskIdKey].(string); hasTaskId {
		paramsMap[taskIdKey] = taskId
	}

	if len(paramsMap) == 0 {
		return nil, errors.New("params map is empty")
	}

	return paramsMap, nil
}

// BuildProjectParameterMapForLegacy builds the parameters map that can be used as an input to GetProjectIdFromParams.
// It is used by the legacy middleware.
func BuildProjectParameterMapForLegacy(query url.Values, vars map[string]string) map[string]string {
	paramsMap := map[string]string{
		projectIdKey: util.CoalesceStrings(append(query["project_id"], query[projectIdKey]...), vars["project_id"], vars[projectIdKey]),
		repoIdKey:    util.CoalesceStrings(append(query["repo_id"], query[repoIdKey]...), vars["repo_id"], vars[repoIdKey]),
		versionIdKey: util.CoalesceStrings(append(query["version_id"], query[versionIdKey]...), vars["version_id"], vars[versionIdKey]),
		patchIdKey:   util.CoalesceStrings(append(query["patch_id"], query[patchIdKey]...), vars["patch_id"], vars[patchIdKey]),
		buildIdKey:   util.CoalesceStrings(append(query["build_id"], query[buildIdKey]...), vars["build_id"], vars[buildIdKey]),
		logIdKey:     util.CoalesceStrings(query["log_id"], vars["log_id"]),
		taskIdKey:    util.CoalesceStrings(append(query["task_id"], query[taskIdKey]...), vars["task_id"], vars[taskIdKey]),
	}
	return paramsMap
}
