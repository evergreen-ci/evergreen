package model

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	projectVarIdKey          = bsonutil.MustHaveTag(ProjectVars{}, "Id")
	projectVarsMapKey        = bsonutil.MustHaveTag(ProjectVars{}, "Vars")
	projectVarsParametersKey = bsonutil.MustHaveTag(ProjectVars{}, "Parameters")
	privateVarsMapKey        = bsonutil.MustHaveTag(ProjectVars{}, "PrivateVars")
	adminOnlyVarsMapKey      = bsonutil.MustHaveTag(ProjectVars{}, "AdminOnlyVars")
)

const (
	ProjectVarsCollection = "project_vars"
	ProjectAWSSSHKeyName  = "__project_aws_ssh_key_name"
	ProjectAWSSSHKeyValue = "__project_aws_ssh_key_value"
)

// ProjectVars holds a map of variables specific to a given project.
// They can be fetched at run time by the agent, so that settings which are
// sensitive or subject to frequent change don't need to be hard-coded into
// yml files.
type ProjectVars struct {

	// Id is the ID of the project.
	Id string `bson:"_id" json:"_id"`

	// Vars is the actual mapping of variable names to values for this project.
	// TODO (DEVPROD-9440): after all project vars are migrated to Parameter
	// Store, remove the BSON tags on this field to ensure project var values
	// are not put in the DB anymore.
	Vars map[string]string `bson:"vars" json:"vars"`

	// Parameters contains the mappings between user-defined project variable
	// names and the parameter name where the variable's value can be found in
	// Parameter Store.
	Parameters ParameterMappings `bson:"parameters,omitempty" json:"parameters,omitempty"`

	// PrivateVars keeps track of which variables are private and should therefore not
	// be returned to the UI server.
	PrivateVars map[string]bool `bson:"private_vars" json:"private_vars"`

	// AdminOnlyVars keeps track of variables that are only accessible by project admins.
	AdminOnlyVars map[string]bool `bson:"admin_only_vars" json:"admin_only_vars"`
}

type ParameterMappings []ParameterMapping

func (pm ParameterMappings) Len() int {
	return len(pm)
}

func (pm ParameterMappings) Less(i, j int) bool {
	return pm[i].Name < pm[j].Name
}

func (pm ParameterMappings) Swap(i, j int) {
	pm[i], pm[j] = pm[j], pm[i]
}

// NameMap returns a map from each name to the full parameter mapping
// information.
func (pm ParameterMappings) NameMap() map[string]ParameterMapping {
	res := map[string]ParameterMapping{}
	for i, m := range pm {
		res[m.Name] = pm[i]
	}
	return res
}

// ParamNameMap returns a map from each parameter name to the full parameter
// mapping information.
func (pm ParameterMappings) ParamNameMap() map[string]ParameterMapping {
	res := make(map[string]ParameterMapping, len(pm))
	for i, m := range pm {
		res[m.ParameterName] = pm[i]
	}
	return res
}
func (pm ParameterMappings) Names() []string {
	res := make([]string, 0, len(pm))
	for _, m := range pm {
		res = append(res, m.Name)
	}
	return res
}

func (pm ParameterMappings) ParamNamesMap() map[string]ParameterMapping {
	res := map[string]ParameterMapping{}
	for i, m := range pm {
		res[m.ParameterName] = pm[i]
	}
	return res
}

func (pm ParameterMappings) ParamNames() []string {
	res := make([]string, 0, len(pm))
	for _, m := range pm {
		res = append(res, m.ParameterName)
	}
	return res
}

// ParameterMapping represents a mapping between a DB field and the location of
// its actual value in Parameter Store. This is used to keep track of where
// sensitive secrets can be found in Parameter Store.
type ParameterMapping struct {
	// Name is the name of the value being stored (e.g. a project variable
	// name).
	Name string `bson:"name" json:"name"`
	// ParameterName is the location where the parameter is kept in Parameter
	// Store.
	ParameterName string `bson:"parameter_name" json:"parameter_name"`
}

type AWSSSHKey struct {
	Name  string
	Value string
}

func FindOneProjectVars(projectId string) (*ProjectVars, error) {
	projectVars := &ProjectVars{}
	q := db.Query(bson.M{projectVarIdKey: projectId})
	err := db.FindOneQ(ProjectVarsCollection, q, projectVars)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return projectVars, nil
}

// FindMergedProjectVars merges vars from the target project's ProjectVars and its parent repo's vars
func FindMergedProjectVars(projectID string) (*ProjectVars, error) {
	project, err := FindBranchProjectRef(projectID)
	if err != nil {
		return nil, errors.Wrapf(err, "getting project '%s'", projectID)
	}
	if project == nil {
		return nil, errors.Errorf("project '%s' does not exist", projectID)
	}

	projectVars, err := FindOneProjectVars(project.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "getting project vars for project '%s'", projectID)
	}
	if !project.UseRepoSettings() {
		return projectVars, nil
	}

	repoVars, err := FindOneProjectVars(project.RepoRefId)
	if err != nil {
		return nil, errors.Wrapf(err, "getting project vars for repo '%s'", project.RepoRefId)
	}
	if repoVars == nil {
		return projectVars, nil
	}
	if projectVars == nil {
		repoVars.Id = project.Id
		return repoVars, nil
	}

	projectVars.MergeWithRepoVars(repoVars)
	return projectVars, nil
}

// UpdateProjectVarsByValue searches all projects who have a variable set to the toReplace input parameter, and replaces all
// matching project variables with the replacement input parameter. If dryRun is set to true, the update is not performed.
// We return a list of keys that were replaced (or, the list of keys that would be replaced in the case that dryRun is true).
// If enabledOnly is set to true, we update only projects that are enabled, and repos.
func UpdateProjectVarsByValue(toReplace, replacement, username string, dryRun, enabledOnly bool) (map[string][]string, error) {
	catcher := grip.NewBasicCatcher()
	matchingProjectVars, err := getVarsByValue(toReplace)
	if err != nil {
		catcher.Wrap(err, "fetching projects with matching value")
	}
	if matchingProjectVars == nil {
		catcher.New("no projects with matching value found")
	}
	changes := map[string][]string{}
	for _, projectVars := range matchingProjectVars {
		for key, val := range projectVars.Vars {
			if val == toReplace {
				identifier := projectVars.Id
				// Don't error if this doesn't work, since we can just use the ID instead, and this may be a repo project.
				pRef, _ := FindBranchProjectRef(projectVars.Id)
				if pRef != nil {
					if enabledOnly && !pRef.Enabled {
						continue
					}
					if pRef.Identifier != "" {
						identifier = pRef.Identifier
					}
				}
				if !dryRun {
					var beforeVars ProjectVars
					err = util.DeepCopy(*projectVars, &beforeVars)
					if err != nil {
						catcher.Wrap(err, "copying project variables")
						continue
					}
					before := ProjectSettings{
						Vars: beforeVars,
					}

					projectVars.Vars[key] = replacement
					err = projectVars.updateSingleVar(key, replacement)
					if err != nil {
						catcher.Wrapf(err, "overwriting variable '%s' for project '%s'", key, projectVars.Id)
						continue
					}

					after := ProjectSettings{
						Vars: *projectVars,
					}

					if err = LogProjectModified(projectVars.Id, username, &before, &after); err != nil {
						catcher.Wrapf(err, "logging project modification for project '%s'", projectVars.Id)
					}
				}
				changes[identifier] = append(changes[identifier], key)
			}
		}
	}
	return changes, catcher.Resolve()
}

func (projectVars *ProjectVars) updateSingleVar(key, val string) error {
	if len(projectVars.Vars) == 0 && len(projectVars.PrivateVars) == 0 &&
		len(projectVars.AdminOnlyVars) == 0 {
		return nil
	}

	return db.Update(
		ProjectVarsCollection,
		bson.M{
			projectVarIdKey: projectVars.Id,
		},
		bson.M{
			"$set": bson.M{
				bsonutil.GetDottedKeyName(projectVarsMapKey, key): val,
			},
		},
	)
}

// CopyProjectVars copies the variables for the first project to the second
func CopyProjectVars(oldProjectId, newProjectId string) error {
	vars, err := FindOneProjectVars(oldProjectId)
	if err != nil {
		return errors.Wrapf(err, "finding variables for project '%s'", oldProjectId)
	}
	if vars == nil {
		vars = &ProjectVars{}
	}

	vars.Id = newProjectId
	// kim: NOTE: it's okay to pass nil here because we're copying from an
	// existing project to a new project that doesn't exist yet. At worst, it'll
	// copy more vars than needed, which is okay.
	_, err = vars.Upsert()
	return errors.Wrapf(err, "inserting variables for project '%s", newProjectId)
}

func SetAWSKeyForProject(projectId string, ssh *AWSSSHKey) error {
	vars, err := FindOneProjectVars(projectId)
	if err != nil {
		return errors.Wrap(err, "getting project vars")
	}
	if vars == nil {
		vars = &ProjectVars{}
	}
	if vars.Vars == nil {
		vars.Vars = map[string]string{}
	}
	if vars.PrivateVars == nil {
		vars.PrivateVars = map[string]bool{}
	}

	vars.Vars[ProjectAWSSSHKeyName] = ssh.Name
	vars.Vars[ProjectAWSSSHKeyValue] = ssh.Value
	vars.PrivateVars[ProjectAWSSSHKeyValue] = true // redact value, but not key name
	_, err = vars.Upsert()
	return errors.Wrap(err, "saving project keys")
}

func GetAWSKeyForProject(projectId string) (*AWSSSHKey, error) {
	vars, err := FindMergedProjectVars(projectId)
	if err != nil {
		return nil, errors.Wrap(err, "getting project vars")
	}
	if vars == nil {
		return nil, errors.New("no variables for project")
	}
	return &AWSSSHKey{
		Name:  vars.Vars[ProjectAWSSSHKeyName],
		Value: vars.Vars[ProjectAWSSSHKeyValue],
	}, nil
}

func (projectVars *ProjectVars) Upsert() (*adb.ChangeInfo, error) {
	// kim; NOTE: it's more efficient to just replace all the vars rather than
	// compute the diff or  looking up before vars in here rather than passing it in
	// because in all likelihood, most logic will have to find the project vars
	// by ID anyways, which for PS would require reading all the project vars
	// into memory. If that happens before this, then loading the project vars
	// is pretty much free here and saves having to plumb the before vars
	// down from the request.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	isPSEnabled, err := isParameterStoreEnabledForProject(ctx, projectVars.Id)
	grip.Error(message.WrapError(err, message.Fields{
		"message":    "could not check if Parameter Store was enabled for project, falling back to assuming it's disabled",
		"project_id": projectVars.Id,
	}))
	if isPSEnabled {
		grip.Error(message.WrapError(projectVars.upsertParameterStore(ctx), message.Fields{
			"message":    "could not upsert project vars into Parameter Store",
			"project_id": projectVars.Id,
		}))
	}

	return db.Upsert(
		ProjectVarsCollection,
		bson.M{
			projectVarIdKey: projectVars.Id,
		},
		bson.M{
			"$set": bson.M{
				projectVarsMapKey:   projectVars.Vars,
				privateVarsMapKey:   projectVars.PrivateVars,
				adminOnlyVarsMapKey: projectVars.AdminOnlyVars,
			},
		},
	)
}

// upsertParameterStore upserts the diff of added/modified/deleted project
// variables into Parameter Store.
func (projectVars *ProjectVars) upsertParameterStore(ctx context.Context) error {
	projectID := projectVars.Id
	after := projectVars
	// kim: TODO: test if the project's vars are synced to PS first. If not,
	// then have to initial sync all of them to PS to initialize the state.
	// kim: TODO: need to think about how to keep params and vars in sync while
	// rollout is ongoing. They may go out of sync if it's disabled.

	before, err := FindOneProjectVars(projectID)
	if err != nil {
		return errors.Wrapf(err, "finding original project vars for project '%s'", projectID)
	}
	if before == nil {
		before = &ProjectVars{}
	}

	varsToUpsert := map[string]string{}
	for varName, afterVal := range after.Vars {
		beforeVal, ok := before.Vars[varName]
		if !ok || beforeVal != afterVal {
			varsToUpsert[varName] = afterVal
		}
	}

	existingParamMappings := before.Parameters.NameMap()

	// kim: NOTE: I'm sure that there's a more elegant way to track the
	// updates/deletes (possibly by absorbing into above logic) and map
	// between the project variable name and param name. But if the logic
	// works, that's good enough for now and it can be cleaned up later.
	// Var name: param name
	paramMappingsToUpdate := map[string]ParameterMapping{}
	pm := evergreen.GetEnvironment().ParameterManager()
	for varName, varValue := range varsToUpsert {
		paramName, paramValue, err := convertVarToParam(projectID, before.Parameters, varName, varValue)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":    "could not get corresponding parameter name for project variable",
				"var_name":   varName,
				"project_id": projectID,
			}))
			continue
		}
		param, err := pm.Put(ctx, paramName, paramValue)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":    "could not put project variable into Parameter Store",
				"var_name":   varName,
				"param_name": paramName,
				"project_id": projectID,
			}))
			continue
		}

		// kim: NOTE: use the resulting param name because it's the full
		// path rather than basename for new parameters. This also accounts
		// for edge cases like project vars being modified and crossing
		// above/below the compression threshold, meaning the parameter name
		// changes (it won't cause any bugs to leave behind the old
		// parameter, even though it's a little wasteful).
		paramMappingsToUpdate[varName] = ParameterMapping{
			Name:          varName,
			ParameterName: param.Name,
		}

		if existingParamMapping, ok := existingParamMappings[varName]; ok && existingParamMapping.ParameterName != param.Name {
			// In a few special edge cases, the project var could already be
			// stored in one parameter name but renamed to a new parameter.
			// For example, if the project var is stored in a parameter
			// named "foo" and then it gets modified to store a very long
			// string, the parameter could be renamed to "foo.gzip" to
			// indicate that it had to be compressed to fit within the
			// parameter 8 KB limitation. If the parameter has been renamed,
			// then the old parameter name is now invalid and should be
			// cleaned up.
			if err := pm.Delete(ctx, existingParamMapping.ParameterName); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":        "could not delete project variable from Parameter Store that was renamed to a different parameter",
					"var_name":       varName,
					"old_param_name": existingParamMapping.ParameterName,
					"new_param_name": param.Name,
					"project_id":     projectID,
				}))
			}
		}
	}

	varsToDelete := map[string]struct{}{}
	namesToDelete := make([]string, 0, len(varsToDelete))
	// Var name : param name
	paramMappingToDelete := map[string]ParameterMapping{}
	for varName := range before.Vars {
		if _, ok := after.Vars[varName]; !ok {
			varsToDelete[varName] = struct{}{}
			if paramMapping, ok := existingParamMappings[varName]; ok {
				namesToDelete = append(namesToDelete, existingParamMappings[varName].ParameterName)
				paramMappingToDelete[varName] = paramMapping
			}
		}
	}
	if len(namesToDelete) > 0 {
		if err := pm.Delete(ctx, namesToDelete...); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":               "could not delete project variables from Parameter Store",
				"vars_to_delete":        varsToDelete,
				"param_names_to_delete": namesToDelete,
				"project_id":            projectID,
			}))
		}
	}

	syncedParamMappings := getSyncedParamMappings(before.Parameters, paramMappingsToUpdate, paramMappingToDelete)

	if _, err := db.Upsert(
		ProjectVarsCollection,
		bson.M{
			projectVarIdKey: projectID,
		},
		bson.M{
			"$set": bson.M{
				projectVarsParametersKey: syncedParamMappings,
			},
		},
	); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":               "could not update parameter mappings for project vars",
			"param_mapping_updates": paramMappingsToUpdate,
			"param_mapping_deletes": paramMappingsToUpdate,
			"project_id":            projectID,
		}))
	}

	after.Parameters = syncedParamMappings

	return nil
}

// kim: NOTE: updated includes added/upserted/renamed parameters.
func getSyncedParamMappings(existingParamMappings []ParameterMapping, updated, deleted map[string]ParameterMapping) []ParameterMapping {
	paramMappings := make(map[string]ParameterMapping, len(existingParamMappings))
	for varName, paramMapping := range updated {
		paramMappings[varName] = ParameterMapping{
			Name:          varName,
			ParameterName: paramMapping.ParameterName,
		}
	}

	for i, paramMapping := range existingParamMappings {
		if _, ok := deleted[paramMapping.Name]; ok {
			continue
		}
		if _, ok := updated[paramMapping.Name]; ok {
			continue
		}
		// If it wasn't added, deleted, or modified, then the mapping is the
		// same as before.
		paramMappings[paramMapping.Name] = existingParamMappings[i]
	}

	res := make(ParameterMappings, 0, len(paramMappings))
	for _, paramMapping := range paramMappings {
		res = append(res, paramMapping)
	}
	// Sort them by variable name so it's in a predictable order.
	sort.Sort(res)

	return res
}

// isParameterStoreEnabledForProject checks if Parameter Store is enabled for a
// project.
func isParameterStoreEnabledForProject(ctx context.Context, projectID string) (bool, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return false, errors.Wrap(err, "getting service flags")
	}
	if flags.ParameterStoreDisabled {
		return false, nil
	}

	projRef, err := FindMergedProjectRef(projectID, "", false)
	if err != nil {
		return false, errors.Wrapf(err, "finding merged project ref '%s'", projectID)
	}
	if projRef != nil {
		return projRef.ParameterStoreEnabled, nil
	}

	// Project vars could tied to a repo instead of branch project, so check the
	// repo as a fallback.
	repoRef, err := FindOneRepoRef(projectID)
	if err != nil {
		return false, errors.Wrapf(err, "finding repo ref '%s'", projectID)
	}
	if repoRef == nil {
		return false, errors.Errorf("project or repo ref '%s' not found", projectID)
	}
	return repoRef.ParameterStoreEnabled, nil
}

func (projectVars *ProjectVars) Insert() error {
	return db.Insert(
		ProjectVarsCollection,
		projectVars,
	)
}

// kim: TODO: update this to also sync with PS as well if enabled (DEVPROD-11973).
// kim: NOTE: ideally only sync the diff.
func (projectVars *ProjectVars) FindAndModify(before *ProjectVars, varsToDelete []string) (*adb.ChangeInfo, error) {
	setUpdate := bson.M{}
	unsetUpdate := bson.M{}
	update := bson.M{}
	if len(projectVars.Vars) == 0 && len(projectVars.PrivateVars) == 0 &&
		len(projectVars.AdminOnlyVars) == 0 && len(varsToDelete) == 0 {
		return nil, nil
	}
	// kim: NOTE: this only sets vars for the modified vars.
	for key, val := range projectVars.Vars {
		setUpdate[bsonutil.GetDottedKeyName(projectVarsMapKey, key)] = val
	}
	for key, val := range projectVars.PrivateVars {
		setUpdate[bsonutil.GetDottedKeyName(privateVarsMapKey, key)] = val
	}
	for key, val := range projectVars.AdminOnlyVars {
		setUpdate[bsonutil.GetDottedKeyName(adminOnlyVarsMapKey, key)] = val
	}
	if len(setUpdate) > 0 {
		update["$set"] = setUpdate
	}

	for _, val := range varsToDelete {
		unsetUpdate[bsonutil.GetDottedKeyName(projectVarsMapKey, val)] = 1
		unsetUpdate[bsonutil.GetDottedKeyName(privateVarsMapKey, val)] = 1
		unsetUpdate[bsonutil.GetDottedKeyName(adminOnlyVarsMapKey, val)] = 1
	}
	if len(unsetUpdate) > 0 {
		update["$unset"] = unsetUpdate
	}
	return db.FindAndModify(
		ProjectVarsCollection,
		bson.M{projectVarIdKey: projectVars.Id},
		nil,
		adb.Change{
			Update:    update,
			ReturnNew: true,
			Upsert:    true,
		},
		projectVars,
	)
}

func (projectVars *ProjectVars) GetVars(t *task.Task) map[string]string {
	vars := map[string]string{}
	isAdmin := shouldGetAdminOnlyVars(t)
	for k, v := range projectVars.Vars {
		if !projectVars.AdminOnlyVars[k] || isAdmin {
			vars[k] = v
		}
	}
	return vars
}

// shouldGetAdminOnlyVars returns true if the task is part of a version that can't be modified by users,
// or if the task was activated by a project admin.
func shouldGetAdminOnlyVars(t *task.Task) bool {
	if utility.StringSliceContains(evergreen.SystemVersionRequesterTypes, t.Requester) {
		return true
	} else if t.ActivatedBy == "" {
		return false
	}
	u, err := user.FindOneById(t.ActivatedBy)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": fmt.Sprintf("problem with fetching user '%s'", t.ActivatedBy),
			"task_id": t.Id,
		}))
		return false
	}
	isAdmin := false
	if u != nil {
		isAdmin = u.HasPermission(gimlet.PermissionOpts{
			Resource:      t.Project,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionProjectSettings,
			RequiredLevel: evergreen.ProjectSettingsEdit.Value,
		})
	}
	return isAdmin
}

func (projectVars *ProjectVars) RedactPrivateVars() *ProjectVars {
	res := &ProjectVars{
		Vars:          map[string]string{},
		PrivateVars:   map[string]bool{},
		AdminOnlyVars: map[string]bool{},
	}
	if projectVars == nil {
		return res
	}
	res.Id = projectVars.Id
	if projectVars.Vars == nil {
		return res
	}
	if projectVars.AdminOnlyVars == nil {
		res.AdminOnlyVars = map[string]bool{}
	}
	if projectVars.PrivateVars == nil {
		res.PrivateVars = map[string]bool{}
	}
	// Redact private variables
	for k, v := range projectVars.Vars {
		if val, ok := projectVars.PrivateVars[k]; ok && val {
			res.Vars[k] = ""
			res.PrivateVars[k] = projectVars.PrivateVars[k]
		} else {
			res.Vars[k] = v
		}
		if val, ok := projectVars.AdminOnlyVars[k]; ok && val {
			res.AdminOnlyVars[k] = projectVars.AdminOnlyVars[k]
		}
	}

	return res
}

func getVarsByValue(val string) ([]*ProjectVars, error) {
	matchingProjects := []*ProjectVars{}
	pipeline := []bson.M{
		{"$addFields": bson.M{projectVarsMapKey: bson.M{"$objectToArray": "$" + projectVarsMapKey}}},
		{"$match": bson.M{bsonutil.GetDottedKeyName(projectVarsMapKey, "v"): val}},
		{"$addFields": bson.M{projectVarsMapKey: bson.M{"$arrayToObject": "$" + projectVarsMapKey}}},
	}

	err := db.Aggregate(ProjectVarsCollection, pipeline, &matchingProjects)

	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return matchingProjects, nil
}

// MergeWithRepoVars merges the project and repo variables
func (projectVars *ProjectVars) MergeWithRepoVars(repoVars *ProjectVars) {
	if projectVars.Vars == nil {
		projectVars.Vars = map[string]string{}
	}
	if projectVars.PrivateVars == nil {
		projectVars.PrivateVars = map[string]bool{}
	}
	if projectVars.AdminOnlyVars == nil {
		projectVars.AdminOnlyVars = map[string]bool{}
	}
	if repoVars == nil {
		return
	}
	// Branch-level vars have priority, so we only need to add a repo vars if it doesn't already exist in the branch
	for key, val := range repoVars.Vars {
		if _, ok := projectVars.Vars[key]; !ok {
			projectVars.Vars[key] = val
			if v, ok := repoVars.PrivateVars[key]; ok {
				projectVars.PrivateVars[key] = v
			}
			if v, ok := repoVars.AdminOnlyVars[key]; ok {
				projectVars.AdminOnlyVars[key] = v
			}
		}
	}
}

// convertVarToParam converts a project variable to its equivalent parameter
// name and value. In particular, it validates that the variable name and value
// fits within parameter constraints and if the name or value doesn't fit in the
// constraints, it attempts to fix minor issues where possible. The return value
// is a valid parameter name and parameter value.
func convertVarToParam(projectID string, pm ParameterMappings, varName, varValue string) (paramName string, paramValue string, err error) {
	if err := validateVarNameCharset(varName); err != nil {
		return "", "", errors.Wrapf(err, "validating project variable name '%s'", varName)
	}

	varsToParams := pm.NameMap()
	m, ok := varsToParams[varName]
	if ok {
		paramName = m.ParameterName
	} else {
		paramName, err = createParamBasenameForVar(varName)
		if err != nil {
			return "", "", errors.Wrapf(err, "creating new parameter name for project variable '%s'", varName)
		}
	}

	paramName, paramValue, err = getCompressedParamForVar(paramName, varValue)
	if err != nil {
		return "", "", errors.Wrapf(err, "getting compressed parameter name and value for project variable '%s'", varName)
	}

	prefix := fmt.Sprintf("%s/", projectID)
	if !strings.Contains(paramName, prefix) {
		paramName = fmt.Sprintf("%s%s", prefix, paramName)
	}

	if err := validateParamNameUnique(pm, varName, paramName); err != nil {
		return "", "", errors.Wrapf(err, "validating parameter name for project variable '%s'", varName)
	}

	return paramName, paramValue, nil
}

// validParamBasename is a regexp representing the valid characters for a
// parameter's base name (i.e. excluding any slash-delimited paths). Valid
// characters for a basename are alphanumerics, underscores, dashes, and
// periods.
var validParamBasename = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// validateVarNameCharset verifies that a project variable name is not empty and
// contains only valid characters. It returns an error if it's empty or contains
// invalid characters that are not allowed in a parameter name.
func validateVarNameCharset(varName string) error {
	if len(varName) == 0 {
		return errors.Errorf("project variable name cannot be empty")
	}
	if !validParamBasename.MatchString(varName) {
		return errors.Errorf("project variable name '%s' contains invalid characters - can only contain alphanumerics, underscores, periods, and dashes", varName)
	}
	if strings.HasSuffix(varName, gzipCompressedParamExtension) {
		// Project variable names should not end in a gzip extension to avoid
		// ambiguity over whether the variable value had to be compressed. The
		// extension is reserved for internal Evergreen use in case there's a
		// project variable that's long enough to require compression to fit
		// within the parameter length limit.
		return errors.Errorf("project variable name '%s' cannot end with '%s'", varName, gzipCompressedParamExtension)
	}
	return nil
}

// validateParamNameUnique verifies if the proposed parameter name to be used is
// unique within a project. It returns an error if the parameter name
// conflicts with an already existing parameter name.
func validateParamNameUnique(pm ParameterMappings, varName, paramName string) error {
	proposedBasename := parameterstore.GetBasename(paramName)
	for _, m := range pm {
		basename := parameterstore.GetBasename(m.ParameterName)
		if m.Name == varName {
			continue
		}
		if basename == proposedBasename {
			// Protect against an edge case where a different project var
			// already exists that has the exact same candidate parameter name.
			// Project vars must map to unique parameter names.
			return errors.Errorf("parameter basename '%s' for project variable '%s' conflicts with existing one for project variable '%s'", proposedBasename, varName, m.Name)
		}
	}

	return nil
}

// createParamBasenameForVar generates a unique parameter basename from a
// project variable name.
func createParamBasenameForVar(varName string) (string, error) {
	paramName := varName

	if strings.HasPrefix(varName, "aws") || strings.HasPrefix(paramName, "ssm") {
		// Parameters cannot start with "aws" or "ssm", adding a prefix
		// (arbitrarily chosen as an underscore) fixes the issue.
		paramName = fmt.Sprintf("_%s", paramName)
	}

	return paramName, nil
}

// gzipCompressedParamExtension is the extension added to the parameter name to
// indicate that the parameter value had to be compressed to fit within the
// parameter length limit.
const gzipCompressedParamExtension = ".gz"

// getCompressedParamForVar returns the parameter name and value for a project
// variable. If the value is too long to be stored in Parameter Store, attempt
// to compress it down to a valid size.
func getCompressedParamForVar(varName, varValue string) (paramName string, paramValue string, err error) {
	if len(varValue) < parameterstore.ParamValueMaxLength {
		return varName, varValue, nil
	}

	compressedValue := bytes.NewBuffer(make([]byte, 0, len(varValue)))
	gzw := gzip.NewWriter(compressedValue)
	if _, err := gzw.Write([]byte(varValue)); err != nil {
		return "", "", errors.Wrap(err, "compressing long project variable value")
	}
	if err := gzw.Close(); err != nil {
		return "", "", errors.Wrap(err, "closing gzip writer after compressing long project variable value")
	}

	if compressedValue.Len() >= parameterstore.ParamValueMaxLength {
		return "", "", errors.Errorf("project variable value exceeds maximum length, even after attempted compression (value is %d bytes, compressed value is %d bytes, maximum is %d bytes)", len(varValue), compressedValue.Len(), parameterstore.ParamValueMaxLength)
	}

	return fmt.Sprintf("%s%s", varName, gzipCompressedParamExtension), compressedValue.String(), nil
}
