package model

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
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
	// This is intentionally not stored in the DB for security reasons. The
	// variables can be fetched from ParameterStore using the Parameters field.
	Vars map[string]string `bson:"-" json:"vars"`

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

// ParameterMappings is a wrapper around a slice of mappings between names and
// their corresponding parameters kept in Parameter Store.
type ParameterMappings []ParameterMapping

// Len returns the number of parameter mappings for the sake of implementing
// sort.Interface.
func (pm ParameterMappings) Len() int {
	return len(pm)
}

// Less returns whether the parameter mapping name at index i must be sorted
// before the parameter mapping name at index j for the sake of implementing
// sort.Interface.
func (pm ParameterMappings) Less(i, j int) bool {
	return pm[i].Name < pm[j].Name
}

// Swap swaps the parameter mappings at indices i and j for the sake of
// implementing sort.Interface.
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

// ParameterNameMap returns a map from each parameter name to the full parameter
// mapping information.
func (pm ParameterMappings) ParameterNameMap() map[string]ParameterMapping {
	res := make(map[string]ParameterMapping, len(pm))
	for i, m := range pm {
		res[m.ParameterName] = pm[i]
	}
	return res
}

// Names returns the names for each parameter mapping.
func (pm ParameterMappings) Names() []string {
	res := make([]string, 0, len(pm))
	for _, m := range pm {
		res = append(res, m.Name)
	}
	return res
}

// ParameterNames returns the parameter names for each parameter mapping.
func (pm ParameterMappings) ParameterNames() []string {
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

// FindOneProjectVars finds the project variables document for a given project
// ID.
func FindOneProjectVars(ctx context.Context, projectId string) (*ProjectVars, error) {
	projectVars := &ProjectVars{}
	q := db.Query(bson.M{projectVarIdKey: projectId})
	err := db.FindOneQContext(ctx, ProjectVarsCollection, q, projectVars)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	projectVarsFromPS, err := projectVars.findParameterStore(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding project vars in Parameter Store")
	}
	return projectVarsFromPS, nil
}

// findParameterStore finds all the project variables from Parameter Store.
func (projectVars *ProjectVars) findParameterStore(ctx context.Context) (*ProjectVars, error) {
	paramMgr := evergreen.GetEnvironment().ParameterManager()

	params, err := paramMgr.GetStrict(ctx, projectVars.Parameters.ParameterNames()...)
	if err != nil {
		return nil, errors.Wrap(err, "getting parameters for project vars")
	}

	varsFromPS := map[string]string{}
	catcher := grip.NewBasicCatcher()
	for _, p := range params {
		varName, varValue, err := convertParamToVar(projectVars.Parameters, p.Name, p.Value)
		if err != nil {
			catcher.Wrapf(err, "parameter '%s'", p.Name)
			continue
		}
		varsFromPS[varName] = varValue
	}

	if catcher.HasErrors() {
		return nil, errors.Wrap(catcher.Resolve(), "converting parameters back to their original project variables")
	}

	projectVars.Vars = varsFromPS

	return projectVars, nil
}

// FindMergedProjectVars merges vars from the target project's ProjectVars and its parent repo's vars
func FindMergedProjectVars(ctx context.Context, projectID string) (*ProjectVars, error) {
	project, err := FindBranchProjectRef(ctx, projectID)
	if err != nil {
		return nil, errors.Wrapf(err, "getting project '%s'", projectID)
	}
	if project == nil {
		return nil, errors.Errorf("project '%s' does not exist", projectID)
	}

	projectVars, err := FindOneProjectVars(ctx, project.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "getting project vars for project '%s'", projectID)
	}
	if !project.UseRepoSettings() {
		return projectVars, nil
	}

	repoVars, err := FindOneProjectVars(ctx, project.RepoRefId)
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

// CopyProjectVars copies the variables for the first project to the second
func CopyProjectVars(ctx context.Context, oldProjectId, newProjectId string) error {
	vars, err := FindOneProjectVars(ctx, oldProjectId)
	if err != nil {
		return errors.Wrapf(err, "finding variables for project '%s'", oldProjectId)
	}
	if vars == nil {
		vars = &ProjectVars{}
	}

	vars.Id = newProjectId
	_, err = vars.Upsert(ctx)
	return errors.Wrapf(err, "inserting variables for project '%s", newProjectId)
}

func SetAWSKeyForProject(ctx context.Context, projectId string, ssh *AWSSSHKey) error {
	vars, err := FindOneProjectVars(ctx, projectId)
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
	_, err = vars.Upsert(ctx)
	return errors.Wrap(err, "saving project keys")
}

func GetAWSKeyForProject(ctx context.Context, projectId string) (*AWSSSHKey, error) {
	vars, err := FindMergedProjectVars(ctx, projectId)
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

// defaultParameterStoreAccessTimeout is the default timeout for accessing
// Parameter Store. In general, the context timeout should prefer to be
// inherited from a higher-level context (e.g. a REST request's context), so
// this timeout should only be used as a last resort if the context cannot
// easily be passed down.
const defaultParameterStoreAccessTimeout = 30 * time.Second

// Upsert creates or updates a project vars document and stores all the project
// variables in the DB. If Parameter Store is enabled for the project, it also
// stores the variables in Parameter Store.
func (projectVars *ProjectVars) Upsert(ctx context.Context) (*adb.ChangeInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultParameterStoreAccessTimeout)
	defer cancel()

	pm, err := projectVars.upsertParameterStore(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "upserting project variables into Parameter Store")
	}
	projectVars.Parameters = *pm

	setUpdate := bson.M{
		privateVarsMapKey:   projectVars.PrivateVars,
		adminOnlyVarsMapKey: projectVars.AdminOnlyVars,
	}
	update := bson.M{}
	if len(projectVars.Parameters) > 0 {
		setUpdate[projectVarsParametersKey] = projectVars.Parameters
	} else {
		update["$unset"] = bson.M{projectVarsParametersKey: 1}
	}
	update["$set"] = setUpdate

	return db.Upsert(
		ctx,
		ProjectVarsCollection,
		bson.M{
			projectVarIdKey: projectVars.Id,
		},
		update,
	)
}

// upsertParameterStore upserts the diff of added/updated/deleted project
// variables into Parameter Store.
func (projectVars *ProjectVars) upsertParameterStore(ctx context.Context) (*ParameterMappings, error) {
	projectID := projectVars.Id
	after := projectVars

	before, err := FindOneProjectVars(ctx, projectID)
	if err != nil {
		return nil, errors.Wrapf(err, "finding original project vars for project '%s'", projectID)
	}
	if before == nil {
		before = &ProjectVars{Id: projectID}
	}

	varsToUpsert, varsToDelete := getProjectVarsDiff(before, after)

	pm, err := projectVars.syncParameterDiff(ctx, before.Parameters, varsToUpsert, varsToDelete)
	if err != nil {
		return nil, errors.Wrap(err, "syncing project vars diff to Parameter Store")
	}

	return pm, nil
}

// syncParameterDiff syncs the diff of project variables to Parameter Store. It
// adds/updates varsToUpsert to Parameter Store, deletes varsToDelete from
// Parameter Store, and updates the project variable parameter mappings.
func (projectVars *ProjectVars) syncParameterDiff(ctx context.Context, pm ParameterMappings, varsToUpsert map[string]string, varsToDelete map[string]struct{}) (*ParameterMappings, error) {
	paramMappingsToUpsert, err := projectVars.upsertParameters(ctx, pm, varsToUpsert)
	if err != nil {
		return nil, errors.Wrap(err, "upserting project variables into Parameter Store")
	}

	paramMappingsToDelete, err := projectVars.deleteParameters(ctx, pm, varsToDelete)
	if err != nil {
		return nil, errors.Wrap(err, "deleting project variables from Parameter Store")
	}

	updatedParamMappings := getUpdatedParamMappings(pm, paramMappingsToUpsert, paramMappingsToDelete)

	return &updatedParamMappings, nil
}

// upsertParameters upserts the parameter mappings for project variables into
// Parameter Store. It returns the parameter mappings for the upserted
// variables.
func (projectVars *ProjectVars) upsertParameters(ctx context.Context, pm ParameterMappings, varsToUpsert map[string]string) (map[string]ParameterMapping, error) {
	projectID := projectVars.Id
	nameToExistingParamMapping := pm.NameMap()
	paramMgr := evergreen.GetEnvironment().ParameterManager()

	paramMappingsToUpsert := map[string]ParameterMapping{}
	catcher := grip.NewBasicCatcher()

	for varName, varValue := range varsToUpsert {
		partialParamName, paramValue, err := convertVarToParam(projectID, pm, varName, varValue)
		if err != nil {
			catcher.Wrapf(err, "converting project variable '%s' to parameter", varName)
			continue
		}
		param, err := paramMgr.Put(ctx, partialParamName, paramValue)
		if err != nil {
			catcher.Wrapf(err, "putting project variable '%s' into Parameter Store", varName)
			continue
		}
		paramName := param.Name

		paramMappingsToUpsert[varName] = ParameterMapping{
			Name:          varName,
			ParameterName: paramName,
		}

		if existingParamMapping, ok := nameToExistingParamMapping[varName]; ok && existingParamMapping.ParameterName != paramName {
			// In a few special edge cases, the project var could already be
			// stored in one parameter name but has to be renamed to a new
			// parameter. For example, if the project var is stored in a
			// parameter named "foo" initially and then the value is updated a
			// very long string, the parameter could be renamed to "foo.gz" to
			// indicate that it had to be compressed to fit within the parameter
			// 8 KB limitation. If the parameter has been renamed, then the old
			// parameter name is now invalid and should be cleaned up.
			if err := paramMgr.Delete(ctx, existingParamMapping.ParameterName); err != nil {
				catcher.Wrapf(err, "deleting project variable '%s' from Parameter Store whose parameter was renamed from '%s' to '%s'", varName, existingParamMapping.ParameterName, paramName)
				continue
			}
		}
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return paramMappingsToUpsert, nil
}

// deleteParameters deletes parameters corresponding to deleted project variables
// from Parameter Store. It returns the parameter mappings for the deleted
// variables.
func (projectVars *ProjectVars) deleteParameters(ctx context.Context, pm ParameterMappings, varsToDelete map[string]struct{}) (map[string]ParameterMapping, error) {
	nameToExistingParamMapping := pm.NameMap()
	paramMappingsToDelete := make(map[string]ParameterMapping, len(varsToDelete))
	for varToDelete := range varsToDelete {
		if paramMapping, ok := nameToExistingParamMapping[varToDelete]; ok {
			paramMappingsToDelete[varToDelete] = paramMapping
		}
	}

	namesToDelete := make([]string, 0, len(paramMappingsToDelete))
	for _, m := range paramMappingsToDelete {
		namesToDelete = append(namesToDelete, m.ParameterName)
	}

	if len(namesToDelete) > 0 {
		paramMgr := evergreen.GetEnvironment().ParameterManager()
		if err := paramMgr.Delete(ctx, namesToDelete...); err != nil {
			return nil, err
		}
	}

	return paramMappingsToDelete, nil
}

// getProjectVarsDiff returns the diff of added/updated/deleted project
// variables between the before and after project variables. It returns the
// variables that have to be upserted and deleted so that before matches after.
func getProjectVarsDiff(before, after *ProjectVars) (upserted map[string]string, deleted map[string]struct{}) {
	varsToUpsert := map[string]string{}
	for varName, afterVal := range after.Vars {
		beforeVal, ok := before.Vars[varName]
		if !ok || beforeVal != afterVal {
			varsToUpsert[varName] = afterVal
		}
	}

	varsToDelete := map[string]struct{}{}
	for varName := range before.Vars {
		if _, ok := after.Vars[varName]; !ok {
			varsToDelete[varName] = struct{}{}
		}
	}

	return varsToUpsert, varsToDelete
}

// getUpdatedParamMappings returns the updated parameter mappings for project
// variables after adding, updating, or deleting parameter mappings. It returns
// the updated parameter mappings.
func getUpdatedParamMappings(original ParameterMappings, upserted, deleted map[string]ParameterMapping) ParameterMappings {
	updatedParamMappings := make(ParameterMappings, 0, len(original))
	for varName := range upserted {
		updatedParamMappings = append(updatedParamMappings, upserted[varName])
	}

	for i, m := range original {
		if _, ok := upserted[m.Name]; ok {
			continue
		}
		if _, ok := deleted[m.Name]; ok {
			continue
		}
		// If it wasn't added, updated, or deleted, then the mapping is the same
		// as it was originally.
		updatedParamMappings = append(updatedParamMappings, original[i])
	}

	// Sort them so the mappings are in a predictable order.
	sort.Sort(updatedParamMappings)

	return updatedParamMappings
}

// Insert creates a new project vars document and stores all the project
// variables in the DB. If Parameter Store is enabled for the project, it also
// stores the variables in Parameter Store.
func (projectVars *ProjectVars) Insert(ctx context.Context) error {
	// This has to be done after inserting the initial document because it
	// upserts the project vars doc. If this ran first, it would cause the DB
	// insert to fail due to the ID already existing.
	ctx, cancel := context.WithTimeout(ctx, defaultParameterStoreAccessTimeout)
	defer cancel()

	pm, err := insertParameterStore(ctx, projectVars)
	if err != nil {
		return errors.Wrap(err, "inserting project vars into Parameter Store")
	}
	projectVars.Parameters = *pm

	return db.Insert(
		ctx,
		ProjectVarsCollection,
		projectVars,
	)
}

// insertParameterStore inserts all project variables into Parameter Store.
func insertParameterStore(ctx context.Context, vars *ProjectVars) (*ParameterMappings, error) {
	before := &ProjectVars{Id: vars.Id}
	after := vars
	varsToUpsert, _ := getProjectVarsDiff(before, after)

	pm, err := vars.syncParameterDiff(ctx, ParameterMappings{}, varsToUpsert, nil)
	if err != nil {
		return nil, errors.Wrap(err, "syncing project vars diff to Parameter Store")
	}

	return pm, nil
}

// FindAndModify is almost the same functionally as Upsert, except that it only
// deletes project vars that are explicitly provided in varsToDelete. In other
// words, even if a project variable is omitted from projectVars, it won't be
// deleted unless that variable is explicitly listed in varsToDelete. If this
// succeeds, projectVars will contain all the project variables, including those
// that were not explicitly modified.
func (projectVars *ProjectVars) FindAndModify(ctx context.Context, varsToDelete []string) (*adb.ChangeInfo, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, defaultParameterStoreAccessTimeout, errors.New("parameter store access timeout"))
	defer cancel()

	pm, err := projectVars.findAndModifyParameterStore(ctx, varsToDelete)
	if err != nil {
		return nil, errors.Wrap(err, "finding and modifying project vars in Parameter Store")
	}
	projectVars.Parameters = *pm

	setUpdate := bson.M{}
	unsetUpdate := bson.M{}
	update := bson.M{}
	if len(projectVars.Vars) == 0 && len(projectVars.PrivateVars) == 0 &&
		len(projectVars.AdminOnlyVars) == 0 && len(projectVars.Parameters) == 0 && len(varsToDelete) == 0 {
		return nil, nil
	}
	for key, val := range projectVars.PrivateVars {
		setUpdate[bsonutil.GetDottedKeyName(privateVarsMapKey, key)] = val
	}
	for key, val := range projectVars.AdminOnlyVars {
		setUpdate[bsonutil.GetDottedKeyName(adminOnlyVarsMapKey, key)] = val
	}
	if len(projectVars.Parameters) > 0 {
		setUpdate[projectVarsParametersKey] = projectVars.Parameters
	} else {
		unsetUpdate[projectVarsParametersKey] = 1
	}
	if len(setUpdate) > 0 {
		update["$set"] = setUpdate
	}

	for _, val := range varsToDelete {
		unsetUpdate[bsonutil.GetDottedKeyName(privateVarsMapKey, val)] = 1
		unsetUpdate[bsonutil.GetDottedKeyName(adminOnlyVarsMapKey, val)] = 1
	}
	if len(unsetUpdate) > 0 {
		update["$unset"] = unsetUpdate
	}

	if len(projectVars.PrivateVars) != 0 && len(projectVars.AdminOnlyVars) != 0 {
		// Initialize the private and admin-only vars maps if they don't exist.
		initializeUpdate := bson.M{}
		originalProjectVars, err := FindOneProjectVars(ctx, projectVars.Id)
		if err != nil {
			return nil, errors.Wrapf(err, "finding original project vars for project '%s'", projectVars.Id)
		}
		if originalProjectVars == nil {
			return nil, errors.Errorf("project vars for project '%s' not found", projectVars.Id)
		}
		if originalProjectVars.PrivateVars == nil {
			initializeUpdate[privateVarsMapKey] = bson.M{}
		}
		if originalProjectVars.AdminOnlyVars == nil {
			initializeUpdate[adminOnlyVarsMapKey] = bson.M{}
		}
		if len(initializeUpdate) > 0 {
			err := db.UpdateContext(ctx,
				ProjectVarsCollection,
				bson.M{projectVarIdKey: projectVars.Id},
				bson.M{"$set": initializeUpdate},
			)
			if err != nil {
				return nil, errors.Wrap(err, "initializing private and admin-only vars in DB")
			}
		}
	}

	change, err := db.FindAndModify(ctx,
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
	if err != nil {
		return nil, errors.Wrap(err, "finding and modifying project vars in DB")
	}

	// FindAndModify is expected to return all the project's vars. However,
	// FindAndModify only receives as input the subset of vars to be modified.
	// Therefore, it's necessary to look up all the vars in Parameter Store
	// after the update to ensure that the returned project vars includes all
	// the unmodified vars.
	projectVarsFromPS, err := projectVars.findParameterStore(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding unmodified project vars in Parameter Store")
	}
	projectVars.Vars = projectVarsFromPS.Vars

	return change, nil
}

// findAndModifyParameterStore is almost the same functionally as Upsert, except
// that it only deletes project vars that are explicitly provided in
// varsToDelete. In other words, even if a project variable is omitted from
// projectVars, it won't be deleted unless that variable is explicitly listed in
// varsToDelete.
func (projectVars *ProjectVars) findAndModifyParameterStore(ctx context.Context, varsToDelete []string) (*ParameterMappings, error) {
	projectID := projectVars.Id

	before, err := FindOneProjectVars(ctx, projectID)
	if err != nil {
		return nil, errors.Wrapf(err, "finding original project vars for project '%s'", projectID)
	}
	if before == nil {
		before = &ProjectVars{Id: projectID}
	}

	// Ignore the vars that are deleted between before and after because
	// FindAndModify only deletes variables that are explicitly specified in
	// varsToDelete.
	after := projectVars
	varsToUpsert, _ := getProjectVarsDiff(before, after)

	varSetToDelete := map[string]struct{}{}
	for _, varName := range varsToDelete {
		varSetToDelete[varName] = struct{}{}
	}

	pm, err := projectVars.syncParameterDiff(ctx, before.Parameters, varsToUpsert, varSetToDelete)
	if err != nil {
		return nil, errors.Wrap(err, "syncing project vars diff to Parameter Store")
	}

	return pm, nil
}

// Clears clears all variables for a project.
func (projectVars *ProjectVars) Clear(ctx context.Context) error {
	projectVars.Vars = map[string]string{}
	projectVars.PrivateVars = map[string]bool{}
	projectVars.AdminOnlyVars = map[string]bool{}

	// Ignore the context cancellation to ensure that the parameters are
	// deleted from Parameter Store and cleared from the database, this should be as
	// 'atomic' as possible.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultParameterStoreAccessTimeout)
	defer cancel()
	if _, err := projectVars.upsertParameterStore(ctx); err != nil {
		return errors.Wrap(err, "clearing project vars from Parameter Store")
	}

	err := db.UpdateContext(ctx, ProjectVarsCollection,
		bson.M{ProjectRefIdKey: projectVars.Id},
		bson.M{
			"$unset": bson.M{
				privateVarsMapKey:        1,
				adminOnlyVarsMapKey:      1,
				projectVarsParametersKey: 1,
			},
		})
	if err != nil {
		return err
	}

	return nil
}

func (projectVars *ProjectVars) GetVars(ctx context.Context, t *task.Task) map[string]string {
	vars := map[string]string{}
	isAdmin := shouldGetAdminOnlyVars(ctx, t)
	for k, v := range projectVars.Vars {
		if !projectVars.AdminOnlyVars[k] || isAdmin {
			vars[k] = v
		}
	}
	return vars
}

// shouldGetAdminOnlyVars returns true if the task is part of a version that can't be modified by users,
// or if the task was activated by a project admin.
func shouldGetAdminOnlyVars(ctx context.Context, t *task.Task) bool {
	if utility.StringSliceContains(evergreen.SystemVersionRequesterTypes, t.Requester) {
		return true
	} else if t.ActivatedBy == "" {
		return false
	}
	u, err := user.FindOneByIdContext(ctx, t.ActivatedBy)
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

// RedactPrivateVars redacts private variable plaintext values and replaces them
// with the empty string.
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

	nameToParamMapping := repoVars.Parameters.NameMap()
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
			if pm, ok := nameToParamMapping[key]; ok {
				projectVars.Parameters = append(projectVars.Parameters, pm)
			}
		}
	}
	// Sort the merged branch project and repo project variables so the mappings
	// are in a predictable order.
	sort.Sort(projectVars.Parameters)
}

// GetVarsParameterPath returns the parameter path for project variables in the
// given project.
func GetVarsParameterPath(projectID string) string {
	// Include a hash of the project ID in the parameter name for uniqueness.
	// The hashing is necessary because project IDs are unique but some
	// existing projects contain characters (e.g. spaces) that are invalid for
	// parameter names.
	hashedProjectID := util.GetSHA256Hash(projectID)
	return fmt.Sprintf("vars/%s", hashedProjectID)
}

// convertVarToParam converts a project variable to its equivalent parameter
// name and value. In particular, it validates that the variable name and value
// fits within parameter constraints and if the name or value doesn't fit in the
// constraints, it attempts to fix minor issues where possible. The return value
// is a valid parameter name and parameter value. This is the inverse operation
// of convertParamToVar.
func convertVarToParam(projectID string, pm ParameterMappings, varName, varValue string) (paramName string, paramValue string, err error) {
	if err := validateVarNameCharset(varName); err != nil {
		return "", "", errors.Wrapf(err, "validating project variable name '%s'", varName)
	}
	if len(varValue) == 0 {
		return "", "", errors.Errorf("project variable '%s' cannot have an empty value", varName)
	}

	prefix := fmt.Sprintf("%s/", GetVarsParameterPath(projectID))

	varsToParams := pm.NameMap()
	m, ok := varsToParams[varName]
	if ok && strings.Contains(m.ParameterName, prefix) {
		// Only reuse the existing parameter name if it exists and already
		// contains the required prefix. If it doesn't have the required prefix,
		// then a new parameter has to be created.
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

	var compressedValue bytes.Buffer
	gzw := gzip.NewWriter(&compressedValue)
	if _, err := gzw.Write([]byte(varValue)); err != nil {
		return "", "", errors.Wrap(err, "compressing long project variable value")
	}
	if err := gzw.Close(); err != nil {
		return "", "", errors.Wrap(err, "closing gzip writer after compressing long project variable value")
	}

	// gzip produces raw binary data, whereas Parameter Store can only handle
	// strings. Encoding it as a base64 string makes it possible to store the
	// gzip-compressed value in Parameter Store.
	compressedBase64Value := base64.StdEncoding.EncodeToString(compressedValue.Bytes())

	if len(compressedBase64Value) >= parameterstore.ParamValueMaxLength {
		return "", "", errors.Errorf("project variable value exceeds maximum length, even after attempted compression (value is %d bytes, compressed value is %d bytes, maximum is %d bytes)", len(varValue), len(compressedBase64Value), parameterstore.ParamValueMaxLength)
	}

	return fmt.Sprintf("%s%s", varName, gzipCompressedParamExtension), compressedBase64Value, nil
}

// convertParamToVar converts a parameter back to its original project variable
// name and value. This is the inverse operation of convertVarToParam.
func convertParamToVar(pm ParameterMappings, paramName, paramValue string) (varName, varValue string, err error) {
	if strings.HasSuffix(paramName, gzipCompressedParamExtension) {
		compressedValue, err := base64.StdEncoding.DecodeString(paramValue)
		if err != nil {
			return "", "", errors.Wrap(err, "decoding base64-encoded compressed parameter value")
		}
		gzr, err := gzip.NewReader(bytes.NewReader(compressedValue))
		if err != nil {
			return "", "", errors.Wrap(err, "creating gzip reader for compressed project variable")
		}
		b, err := io.ReadAll(gzr)
		if err != nil {
			return "", "", errors.Wrap(err, "decoding gzip-compressed parameter to project variable")
		}
		varValue = string(b)
	} else {
		varValue = paramValue
	}

	m, ok := pm.ParameterNameMap()[paramName]
	if !ok {
		return "", "", errors.Errorf("cannot find project variable name corresponding to parameter '%s'", paramName)
	}
	varName = m.Name
	if varName == "" {
		return "", "", errors.Errorf("project variable name corresponding to parameter '%s' exists but is empty", paramName)
	}

	return varName, varValue, nil
}
