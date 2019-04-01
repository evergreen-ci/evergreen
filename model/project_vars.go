package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	projectVarIdKey   = bsonutil.MustHaveTag(ProjectVars{}, "Id")
	projectVarsMapKey = bsonutil.MustHaveTag(ProjectVars{}, "Vars")
	privateVarsMapKey = bsonutil.MustHaveTag(ProjectVars{}, "PrivateVars")
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
}

type AWSSSHKey struct {
	Name  string
	Value string
}

func FindOneProjectVars(projectId string) (*ProjectVars, error) {
	projectVars := &ProjectVars{}
	err := db.FindOne(
		ProjectVarsCollection,
		bson.M{
			projectVarIdKey: projectId,
		},
		db.NoProjection,
		db.NoSort,
		projectVars,
	)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return projectVars, nil
}

func SetAWSKeyForProject(projectId string, ssh *AWSSSHKey) error {
	vars, err := FindOneProjectVars(projectId)
	if err != nil {
		return errors.Wrap(err, "problem getting project vars")
	}
	vars.Vars[ProjectAWSSSHKeyName] = ssh.Name
	vars.Vars[ProjectAWSSSHKeyValue] = ssh.Value
	vars.PrivateVars[ProjectAWSSSHKeyValue] = true // redact value, but not key name
	_, err = vars.Upsert()
	return errors.Wrap(err, "problem saving project keys")
}

func GetAWSKeyForProject(projectId string) (*AWSSSHKey, error) {
	vars, err := FindOneProjectVars(projectId)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting project vars")
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
				projectVarsMapKey: projectVars.Vars,
				privateVarsMapKey: projectVars.PrivateVars,
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

func (projectVars *ProjectVars) RedactPrivateVars() {
	if projectVars != nil &&
		projectVars.Vars != nil &&
		projectVars.PrivateVars != nil {
		// Redact private variables
		for k := range projectVars.Vars {
			if _, ok := projectVars.PrivateVars[k]; ok {
				projectVars.Vars[k] = ""
			}
		}
	}
}
