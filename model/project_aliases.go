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
	variantsKey  = bsonutil.MustHaveTag(ProjectAlias{}, "Variants")
	tasksKey     = bsonutil.MustHaveTag(ProjectAlias{}, "Tasks")
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
// testing uses a special alias, "github_pull_request" to determine the default
// variants and tasks to run in a patch build.
//
// An alias can be specified multiple times. The resulting variant/task
// combinations are the union of the aliases. For example, a user might set the
// following:
//
// ALIAS                  VARIANTS          TASKS
// github_pull_request    .*linux.*         .*test.*
// github_pull_request    ^ubuntu1604.*$    ^comppile.*$
//
// This will cause a GitHub pull request to create and finalize a patch which runs
// all tasks containing the string “test” on all variants containing the string
// “linux”; and to run all tasks beginning with the string “compile” to run on all
// variants beginning with the string “ubuntu1604”.
type ProjectAlias struct {
	ID        bson.ObjectId `bson:"_id"`
	ProjectID string        `bson:"project_id"`
	Alias     string        `bson:"alias"`
	Variants  string        `bson:"variants"`
	Tasks     string        `bson:"tasks"`
}

// FindProjectAliases finds aliases with a given name for a project.
func FindProjectAliases(projectID, alias string) ([]ProjectAlias, error) {
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

// Insert adds a project alias to the database.
func (p *ProjectAlias) Insert() error {
	if p.ID == "" {
		p.ID = bson.NewObjectId()
	}
	err := db.Insert(ProjectAliasCollection, p)
	if err != nil {
		return errors.Wrapf(err, "failed to insert project alias", p.ID)
	}
	return nil
}

// Remove removes a matching project alias from the database.
func (p *ProjectAlias) Remove() error {
	if p.ID == "" {
		return errors.New("can't remove project alias with empty id")
	}
	err := db.Remove(ProjectAliasCollection, db.Query(bson.M{idKey: p.ID}))
	if err != nil {
		return errors.Wrapf(err, "failed to remove project alias %s", p.ID)
	}
	return nil
}
