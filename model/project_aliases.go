package model

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

var (
	idKey          = bsonutil.MustHaveTag(ProjectAlias{}, "ID")
	projectIDKey   = bsonutil.MustHaveTag(ProjectAlias{}, "ProjectID")
	aliasKey       = bsonutil.MustHaveTag(ProjectAlias{}, "Alias")
	variantKey     = bsonutil.MustHaveTag(ProjectAlias{}, "Variant")
	taskKey        = bsonutil.MustHaveTag(ProjectAlias{}, "Task")
	variantTagsKey = bsonutil.MustHaveTag(ProjectAlias{}, "VariantTags")
	taskTagsKey    = bsonutil.MustHaveTag(ProjectAlias{}, "TaskTags")
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
	ID          mgobson.ObjectId `bson:"_id" json:"_id"`
	ProjectID   string           `bson:"project_id" json:"project_id"`
	Alias       string           `bson:"alias" json:"alias"`
	Variant     string           `bson:"variant,omitempty" json:"variant"`
	VariantTags []string         `bson:"variant_tags,omitempty" json:"variant_tags"`
	Task        string           `bson:"task,omitempty" json:"task"`
	TaskTags    []string         `bson:"tags,omitempty" json:"tags"`
}

type ProjectAliases []ProjectAlias

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

// IsValidId returns whether the supplied Id is a valid patch doc id (BSON ObjectId).
func IsValidId(id string) bool {
	return mgobson.IsObjectIdHex(id)
}

// NewId constructs a valid patch Id from the given hex string.
func NewId(id string) mgobson.ObjectId { return mgobson.ObjectIdHex(id) }

func (p *ProjectAlias) Upsert() error {
	if len(p.ProjectID) == 0 {
		return errors.New("empty project ID")
	}
	if p.ID.Hex() == "" {
		p.ID = mgobson.NewObjectId()
	}
	update := bson.M{
		aliasKey:       p.Alias,
		projectIDKey:   p.ProjectID,
		variantKey:     p.Variant,
		variantTagsKey: p.VariantTags,
		taskTagsKey:    p.TaskTags,
		taskKey:        p.Task,
	}

	_, err := db.Upsert(ProjectAliasCollection, bson.M{
		idKey: p.ID,
	}, bson.M{"$set": update})
	if err != nil {
		return errors.Wrapf(err, "failed to insert project alias '%s'", p.ID)
	}
	return nil
}

func UpsertAliasesForProject(aliases []ProjectAlias, projectId string) error {
	catcher := grip.NewBasicCatcher()
	for i := range aliases {
		if aliases[i].ProjectID != projectId { // new project, so we need a new document (new ID)
			aliases[i].ProjectID = projectId
			aliases[i].ID = ""
		}
		catcher.Add(aliases[i].Upsert())
	}
	return catcher.Resolve()
}

// RemoveProjectAlias removes a project alias with the given document ID from the
// database.
func RemoveProjectAlias(id string) error {
	if id == "" {
		return errors.New("can't remove project alias with empty id")
	}
	err := db.Remove(ProjectAliasCollection, bson.M{idKey: mgobson.ObjectIdHex(id)})
	if err != nil {
		return errors.Wrapf(err, "failed to remove project alias %s", id)
	}
	return nil
}

func (a ProjectAliases) HasMatchingVariant(variant string, variantTags []string) (bool, error) {
	for _, alias := range a {
		variantRegex, err := regexp.Compile(alias.Variant)
		if err != nil {
			return false, errors.Wrapf(err, "unable to compile regex %s", variantRegex)
		}
		if isValidRegexOrTag(variant, alias.Variant, variantTags, alias.VariantTags, variantRegex) {
			return true, nil
		}
	}
	return false, nil
}

func (a ProjectAliases) HasMatchingTask(variant string, variantTags []string, t *ProjectTask) (bool, error) {
	if t == nil {
		return false, errors.New("no task found")
	}
	for _, alias := range a {
		variantRegex, err := regexp.Compile(alias.Variant)
		if err != nil {
			return false, errors.Wrapf(err, "unable to compile regex %s", variantRegex)
		}
		if isValidRegexOrTag(variant, alias.Variant, variantTags, alias.VariantTags, variantRegex) {
			taskRegex, err := regexp.Compile(alias.Task)
			if err != nil {
				return false, errors.Wrapf(err, "unable to compile regex %s", taskRegex)
			}
			if isValidRegexOrTag(t.Name, alias.Task, t.Tags, alias.TaskTags, taskRegex) {
				return true, nil
			}
		}
	}
	return false, nil
}

func isValidRegexOrTag(curItem, aliasRegex string, curTags, aliasTags []string, regexp *regexp.Regexp) bool {
	isValidRegex := aliasRegex != "" && regexp.MatchString(curItem)

	isValidTag := false
	for _, tag := range aliasTags {
		if util.StringSliceContains(curTags, tag) {
			isValidTag = true
			break
		}
		// a negated tag
		if len(tag) > 0 && tag[0] == '!' && !util.StringSliceContains(curTags, tag[1:]) {
			isValidTag = true
			break
		}
	}

	return isValidRegex || isValidTag
}

func ValidateProjectAliases(aliases []ProjectAlias, aliasType string) []string {
	errs := []string{}
	for i, pd := range aliases {
		if strings.TrimSpace(pd.Alias) == "" {
			errs = append(errs, fmt.Sprintf("%s: alias name #%d can't be empty string", aliasType, i+1))
		}
		if (strings.TrimSpace(pd.Variant) == "") == (len(pd.VariantTags) == 0) {
			errs = append(errs, fmt.Sprintf("%s: must specify exactly one of variant regex or variant tags on line #%d", aliasType, i+1))
		}
		if (strings.TrimSpace(pd.Task) == "") == (len(pd.TaskTags) == 0) {
			errs = append(errs, fmt.Sprintf("%s: must specify exactly one of task regex or task tags on line #%d", aliasType, i+1))
		}

		if _, err := regexp.Compile(pd.Variant); err != nil {
			errs = append(errs, fmt.Sprintf("%s: variant regex #%d is invalid", aliasType, i+1))
		}
		if _, err := regexp.Compile(pd.Task); err != nil {
			errs = append(errs, fmt.Sprintf("%s: task regex #%d is invalid", aliasType, i+1))
		}
	}

	return errs
}
