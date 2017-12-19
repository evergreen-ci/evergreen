package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	projectVarIdKey     = bsonutil.MustHaveTag(ProjectVars{}, "Id")
	projectVarsMapKey   = bsonutil.MustHaveTag(ProjectVars{}, "Vars")
	privateVarsMapKey   = bsonutil.MustHaveTag(ProjectVars{}, "PrivateVars")
	patchDefinitionsKey = bsonutil.MustHaveTag(ProjectVars{}, "PatchDefinitions")
	githubWebhookIDKey  = bsonutil.MustHaveTag(ProjectVars{}, "GithubHookID")
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

	// PatchDefinitions contains regexes that are used to determine which
	// combinations of variants and tasks should be run in a patch build.
	PatchDefinitions []PatchDefinition `bson:"patch_definitions" json:"patch_definitions"`

	// GithubHookID is the unique number for the Github Hook configuration
	// of this repository
	GithubHookID int `bson:"github_hook_id,omitempty" json:"github_hook_id,omitempty"`
}

type PatchDefinition struct {
	Alias   string `bson:"alias" json:"alias"`
	Variant string `bson:"variant" json:"variant"`
	Task    string `bson:"task" json:"task"`
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

func FindAllProjectAliases(projectId string) ([]PatchDefinition, error) {
	vars, err := FindOneProjectVars(projectId)
	if err != nil {
		return nil, err
	}
	if vars == nil {
		return nil, nil
	}
	aliases := vars.PatchDefinitions
	if len(aliases) == 0 {
		return nil, nil
	}

	return aliases, nil
}

func FindOneProjectAlias(projectId string, alias string) ([]PatchDefinition, error) {
	vars, err := FindOneProjectVars(projectId)
	if err != nil {
		return nil, err
	}
	if vars == nil {
		return nil, nil
	}

	aliases := []PatchDefinition{}
	for _, d := range vars.PatchDefinitions {
		if d.Alias == alias {
			aliases = append(aliases, d)
		}
	}
	if len(aliases) == 0 {
		return nil, nil
	}

	return aliases, nil
}

func (projectVars *ProjectVars) Upsert() (*mgo.ChangeInfo, error) {
	return db.Upsert(
		ProjectVarsCollection,
		bson.M{
			projectVarIdKey: projectVars.Id,
		},
		bson.M{
			"$set": bson.M{
				projectVarsMapKey:   projectVars.Vars,
				privateVarsMapKey:   projectVars.PrivateVars,
				patchDefinitionsKey: projectVars.PatchDefinitions,
				githubWebhookIDKey:  projectVars.GithubHookID,
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
