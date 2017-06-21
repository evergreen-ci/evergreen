package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ProjectVarIdKey   = bsonutil.MustHaveTag(ProjectVars{}, "Id")
	ProjectVarsMapKey = bsonutil.MustHaveTag(ProjectVars{}, "Vars")
	PrivateVarsMapKey = bsonutil.MustHaveTag(ProjectVars{}, "PrivateVars")
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
}

func FindOneProjectVars(projectId string) (*ProjectVars, error) {
	projectVars := &ProjectVars{}
	err := db.FindOne(
		ProjectVarsCollection,
		bson.M{
			ProjectVarIdKey: projectId,
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
			ProjectVarIdKey: projectVars.Id,
		},
		bson.M{
			"$set": bson.M{
				ProjectVarsMapKey: projectVars.Vars,
				PrivateVarsMapKey: projectVars.PrivateVars,
			},
		},
	)
}

func (projectVars *ProjectVars) Insert() error {
	return db.Insert(
		ProjectVarsCollection,
		bson.M{
			ProjectVarIdKey:   projectVars.Id,
			ProjectVarsMapKey: projectVars.Vars,
			PrivateVarsMapKey: projectVars.PrivateVars,
		},
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
