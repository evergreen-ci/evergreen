package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
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
	projectVarIdKey     = bsonutil.MustHaveTag(ProjectVars{}, "Id")
	projectVarsMapKey   = bsonutil.MustHaveTag(ProjectVars{}, "Vars")
	privateVarsMapKey   = bsonutil.MustHaveTag(ProjectVars{}, "PrivateVars")
	adminOnlyVarsMapKey = bsonutil.MustHaveTag(ProjectVars{}, "AdminOnlyVars")
)

const (
	ProjectVarsCollection = "project_vars"
	ProjectAWSSSHKeyName  = "__project_aws_ssh_key_name"
	ProjectAWSSSHKeyValue = "__project_aws_ssh_key_value"
)

//ProjectVars holds a map of variables specific to a given project.
//They can be fetched at run time by the agent, so that settings which are
//sensitive or subject to frequent change don't need to be hard-coded into
//yml files.
type ProjectVars struct {

	//Should match the identifier of the project it refers to
	Id string `bson:"_id" json:"_id"`

	//The actual mapping of variables for this project
	Vars map[string]string `bson:"vars" json:"vars"`

	//PrivateVars keeps track of which variables are private and should therefore not
	//be returned to the UI server.
	PrivateVars map[string]bool `bson:"private_vars" json:"private_vars"`

	// AdminOnlyVars keeps track of variables that are only accessible by project admins
	AdminOnlyVars map[string]bool `bson:"admin_only_vars" json:"admin_only_vars"`
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
		return nil, errors.Wrapf(err, "problem getting project '%s'", projectID)
	}
	if project == nil {
		return nil, errors.Errorf("project '%s' does not exist", projectID)
	}

	projectVars, err := FindOneProjectVars(project.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "problem getting project vars for project '%s'", projectID)
	}
	if !project.UseRepoSettings() {
		return projectVars, nil
	}

	repoVars, err := FindOneProjectVars(project.RepoRefId)
	if err != nil {
		return nil, errors.Wrapf(err, "problem getting project vars for repo '%s'", project.RepoRefId)
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

func UpdateProjectVarsByValue(toReplace, replacement, username string, dryRun bool) (map[string][]string, error) {
	catcher := grip.NewBasicCatcher()
	matchingProjects, err := GetVarsByValue(toReplace)
	if err != nil {
		catcher.Wrap(err, "failed to fetch projects with matching value")
	}
	if matchingProjects == nil {
		catcher.New("no projects with matching value found")
	}
	changes := map[string][]string{}
	for _, project := range matchingProjects {
		for key, val := range project.Vars {
			if val == toReplace {
				if !dryRun {
					originalVars := make(map[string]string)
					for k, v := range project.Vars {
						originalVars[k] = v
					}
					before := ProjectSettings{
						Vars: ProjectVars{
							Id:   project.Id,
							Vars: originalVars,
						},
					}

					project.Vars[key] = replacement
					_, err = project.Upsert()
					if err != nil {
						catcher.Wrapf(err, "problem overwriting variables for project '%s'", project.Id)
					}

					after := ProjectSettings{
						Vars: ProjectVars{
							Id:   project.Id,
							Vars: project.Vars,
						},
					}

					if err = LogProjectModified(project.Id, username, &before, &after); err != nil {
						catcher.Wrapf(err, "Error logging project modification for project '%s'", project.Id)
					}
				}
				changes[project.Id] = append(changes[project.Id], key)
			}
		}
	}
	return changes, catcher.Resolve()
}

// CopyProjectVars copies the variables for the first project to the second
func CopyProjectVars(oldProjectId, newProjectId string) error {
	vars, err := FindOneProjectVars(oldProjectId)
	if err != nil {
		return errors.Wrapf(err, "error finding variables for project '%s'", oldProjectId)
	}
	vars.Id = newProjectId
	_, err = vars.Upsert()
	return errors.Wrapf(err, "error inserting variables for project '%s", newProjectId)
}

func SetAWSKeyForProject(projectId string, ssh *AWSSSHKey) error {
	vars, err := FindOneProjectVars(projectId)
	if err != nil {
		return errors.Wrap(err, "problem getting project vars")
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
	return errors.Wrap(err, "problem saving project keys")
}

func GetAWSKeyForProject(projectId string) (*AWSSSHKey, error) {
	vars, err := FindMergedProjectVars(projectId)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting project vars")
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

func (projectVars *ProjectVars) Insert() error {
	return db.Insert(
		ProjectVarsCollection,
		projectVars,
	)
}

func (projectVars *ProjectVars) FindAndModify(varsToDelete []string) (*adb.ChangeInfo, error) {
	setUpdate := bson.M{}
	unsetUpdate := bson.M{}
	update := bson.M{}
	if len(projectVars.Vars) == 0 && len(projectVars.PrivateVars) == 0 &&
		len(projectVars.AdminOnlyVars) == 0 && len(varsToDelete) == 0 {
		return nil, nil
	}
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
	isAdmin := projectVars.ShouldGetAdminOnlyVars(t)
	for k, v := range projectVars.Vars {
		if !projectVars.AdminOnlyVars[k] || isAdmin {
			vars[k] = v
		}
	}
	return vars
}

func (projectVars *ProjectVars) ShouldGetAdminOnlyVars(t *task.Task) bool {
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

func GetVarsByValue(val string) ([]*ProjectVars, error) {
	matchingProjects := []*ProjectVars{}
	filter := fmt.Sprintf("function() { for (var field in this.vars) { if (this.vars[field] == \"%s\") return true; } return false; }", val)
	q := db.Query(bson.M{"$where": filter})
	err := db.FindAllQ(ProjectVarsCollection, q, &matchingProjects)

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
