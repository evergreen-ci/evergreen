package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

var (
	idKey        = bsonutil.MustHaveTag(ProjectAlias{}, "ID")
	projectIDKey = bsonutil.MustHaveTag(ProjectAlias{}, "ProjectID")
	aliasKey     = bsonutil.MustHaveTag(ProjectAlias{}, "Alias")
	variantKey   = bsonutil.MustHaveTag(ProjectAlias{}, "Variant")
	taskKey      = bsonutil.MustHaveTag(ProjectAlias{}, "Task")
	tagsKey      = bsonutil.MustHaveTag(ProjectAlias{}, "Tags")
)

const (
	ProjectAliasCollection = "project_aliases"
)

// ProjectAlias defines a single alias mapping an alias name to two regexes which
// define the variants and tasks for the alias. Users can use these aliases for
// operations within the system.
//
// For example, a user can specify that alias with the CLI tool so that a project
// admin can define a set of default builders for patch builds. Pull request
// testing uses a special alias, "__github" to determine the default
// variants and tasks to run in a patch build.
//
// An alias can be specified multiple times. The resulting variant/task
// combinations are the union of the aliases. For example, a user might set the
// following:
//
// ALIAS                  VARIANTS          TASKS
// __github               .*linux.*         .*test.*
// __github               ^ubuntu1604.*$    ^compile.*$
//
// This will cause a GitHub pull request to create and finalize a patch which runs
// all tasks containing the string “test” on all variants containing the string
// “linux”; and to run all tasks beginning with the string “compile” to run on all
// variants beginning with the string “ubuntu1604”.
type ProjectAlias struct {
	ID        bson.ObjectId `bson:"_id" json:"_id"`
	ProjectID string        `bson:"project_id" json:"project_id"`
	Alias     string        `bson:"alias" json:"alias"`
	Variant   string        `bson:"variant" json:"variant"`
	Task      string        `bson:"task,omitempty" json:"task"`
	Tags      []string      `bson:"tags,omitempty" json:"tags"`
}

// FindAliasesForProject fetches all aliases for a given project
func FindAliasesForProject(projectID string) ([]ProjectAlias, error) {
	out := []ProjectAlias{}
	q := db.Query(bson.M{
		projectIDKey: projectID,
	})
	err := db.FindAllQ(ProjectAliasCollection, q, &out)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// FindAliasInProject finds all aliases with a given name for a project.
func FindAliasInProject(projectID, alias string) ([]ProjectAlias, error) {
	var out []ProjectAlias
	q := db.Query(bson.M{
		projectIDKey: projectID,
		aliasKey:     alias,
	})
	err := db.FindAllQ(ProjectAliasCollection, q, &out)
	if err != nil {
		return []ProjectAlias{}, errors.Wrap(err, "error finding project aliases")
	}
	return out, nil
}

func (p *ProjectAlias) Upsert() error {
	if len(p.ProjectID) == 0 {
		return errors.New("empty project ID")
	}
	if p.ID.Hex() == "" {
		p.ID = bson.NewObjectId()
	}
	update := bson.M{
		aliasKey:     p.Alias,
		projectIDKey: p.ProjectID,
		variantKey:   p.Variant,
	}
	if p.Task != "" {
		update[taskKey] = p.Task
	}
	if len(p.Tags) > 0 {
		update[tagsKey] = p.Tags
	}

	_, err := db.Upsert(ProjectAliasCollection, bson.M{
		idKey: p.ID,
	}, bson.M{"$set": update})
	if err != nil {
		return errors.Wrapf(err, "failed to insert project alias '%s'", p.ID)
	}
	return nil
}

// RemoveProjectAlias removes a project alias with the given document ID from the
// database.
func RemoveProjectAlias(id string) error {
	if id == "" {
		return errors.New("can't remove project alias with empty id")
	}
	err := db.Remove(ProjectAliasCollection, db.Query(bson.M{idKey: bson.ObjectIdHex(id)}))
	if err != nil {
		return errors.Wrapf(err, "failed to remove project alias %s", id)
	}
	return nil
}
