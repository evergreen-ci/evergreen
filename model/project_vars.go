package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	projectVarIdKey   = bsonutil.MustHaveTag(ProjectVars{}, "Id")
	projectVarsMapKey = bsonutil.MustHaveTag(ProjectVars{}, "Vars")
	privateVarsMapKey = bsonutil.MustHaveTag(ProjectVars{}, "PrivateVars")
	githubHookIDKey   = bsonutil.MustHaveTag(ProjectVars{}, "GithubHookID")
)

const (
	ProjectVarsCollection = "project_vars"
)

//ProjectVars holds a map of variables specific to a given project.
//They can be fetched at run time by the agent, so that settings which are
//sensitive or subject to frequent change don't need to be hard-coded into
//yml files.
type ProjectVars struct {

	//Should match the _id in the project it refers to
	Id string `bson:"_id" json:"_id"`

	//The actual mapping of variables for this project
	Vars map[string]string `bson:"vars" json:"vars"`

	//PrivateVars keeps track of which variables are private and should therefore not
	//be returned to the UI server.
	PrivateVars map[string]bool `bson:"private_vars" json:"private_vars"`

	// GithubHookID is the unique number for the Github Hook configuration
	// of this repository
	GithubHookID int `bson:"github_hook_id,omitempty" json:"github_hook_id,omitempty"`
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
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return projectVars, nil
}

func (projectVars *ProjectVars) Upsert() (*mgo.ChangeInfo, error) {
	return db.Upsert(
		ProjectVarsCollection,
		bson.M{
			projectVarIdKey: projectVars.Id,
		},
		bson.M{
			"$set": bson.M{
				projectVarsMapKey: projectVars.Vars,
				privateVarsMapKey: projectVars.PrivateVars,
				githubHookIDKey:   projectVars.GithubHookID,
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
