package model

import (
	"10gen.com/mci/db"
	"10gen.com/mci/db/bsonutil"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

// The ProjectRef struct contains general information, independent of any
// revision control system, needed to track a given project
type ProjectRef struct {
	Owner       string `bson:"owner_name" json:"owner_name" yaml:"owner"`
	Repo        string `bson:"repo_name" json:"repo_name" yaml:"repo"`
	Branch      string `bson:"branch_name" json:"branch_name" yaml:"branch"`
	RepoKind    string `bson:"repo_kind" json:"repo_kind" yaml:"repokind"`
	Enabled     bool   `bson:"enabled" json:"enabled" yaml:"enabled"`
	Private     bool   `bson:"private" json:"private" yaml:"private"`
	BatchTime   int    `bson:"batch_time" json:"batch_time" yaml:"batchtime"`
	RemotePath  string `bson:"remote_path" json:"remote_path" yaml:"remote_path"`
	Identifier  string `bson:"identifier" json:"identifier" yaml:"identifier"`
	DisplayName string `bson:"display_name" json:"display_name" yaml:"display_name"`
	Remote      bool   `bson:"remote" json:"remote" yaml:"remote"`
	//Tracked determines whether or not the project is discoverable in the UI
	Tracked bool `bson:"tracked" json:"tracked"`
}

var (
	// bson fields for the ProjectRef struct
	ProjectRefOwnerKey       = bsonutil.MustHaveTag(ProjectRef{}, "Owner")
	ProjectRefRepoKey        = bsonutil.MustHaveTag(ProjectRef{}, "Repo")
	ProjectRefBranchKey      = bsonutil.MustHaveTag(ProjectRef{}, "Branch")
	ProjectRefRepoKindKey    = bsonutil.MustHaveTag(ProjectRef{}, "RepoKind")
	ProjectRefEnabledKey     = bsonutil.MustHaveTag(ProjectRef{}, "Enabled")
	ProjectRefPrivateKey     = bsonutil.MustHaveTag(ProjectRef{}, "Private")
	ProjectRefBatchTimeKey   = bsonutil.MustHaveTag(ProjectRef{}, "BatchTime")
	ProjectRefIdentifierKey  = bsonutil.MustHaveTag(ProjectRef{}, "Identifier")
	ProjectRefDisplayNameKey = bsonutil.MustHaveTag(ProjectRef{}, "DisplayName")
	ProjectRefRemoteKey      = bsonutil.MustHaveTag(ProjectRef{}, "Remote")
	ProjectRefRemotePathKey  = bsonutil.MustHaveTag(ProjectRef{}, "RemotePath")
	ProjectRefTrackedKey     = bsonutil.MustHaveTag(ProjectRef{}, "Tracked")
)

const (
	ProjectRefCollection = "project_ref"
)

func (projectRef *ProjectRef) Insert() error {
	return db.Insert(ProjectRefCollection, projectRef)
}

// FindOneProjectRef gets a project ref given the owner name, the repo
// name and the project name
func FindOneProjectRef(identifier string) (*ProjectRef, error) {
	projectRef := &ProjectRef{}
	err := db.FindOne(
		ProjectRefCollection,
		bson.M{
			ProjectRefIdentifierKey: identifier,
		},
		db.NoProjection,
		db.NoSort,
		projectRef,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return projectRef, err
}

// FindAllTrackedProjectRefs returns all project refs in the db
// that are currently being tracked (i.e. their project files
// still exist)
func FindAllTrackedProjectRefs() ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}
	err := db.FindAll(
		ProjectRefCollection,
		bson.M{ProjectRefTrackedKey: true},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
		&projectRefs,
	)
	return projectRefs, err
}

// FindAllProjectRefs returns all project refs in the db
func FindAllProjectRefs() ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}
	err := db.FindAll(
		ProjectRefCollection,
		bson.M{},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
		&projectRefs,
	)
	return projectRefs, err
}

// UntrackStaleProjectRefs sets all project_refs in the db not in the array
// of project identifiers to "untracked."
func UntrackStaleProjectRefs(activeProjects []string) error {
	_, err := db.UpdateAll(
		ProjectRefCollection,
		bson.M{ProjectRefIdentifierKey: bson.M{
			"$nin": activeProjects,
		}},
		bson.M{"$set": bson.M{
			ProjectRefTrackedKey: false,
		}},
	)
	return err
}

// Upsert updates the project ref in the db if an entry already exists,
// overwriting the existing ref. If no project ref exists, one is created
func (projectRef *ProjectRef) Upsert() error {
	_, err := db.Upsert(
		ProjectRefCollection,
		bson.M{
			ProjectRefIdentifierKey: projectRef.Identifier,
		},
		bson.M{
			"$set": bson.M{
				ProjectRefRepoKindKey:    projectRef.RepoKind,
				ProjectRefEnabledKey:     projectRef.Enabled,
				ProjectRefPrivateKey:     projectRef.Private,
				ProjectRefBatchTimeKey:   projectRef.BatchTime,
				ProjectRefOwnerKey:       projectRef.Owner,
				ProjectRefRepoKey:        projectRef.Repo,
				ProjectRefBranchKey:      projectRef.Branch,
				ProjectRefDisplayNameKey: projectRef.DisplayName,
				ProjectRefTrackedKey:     projectRef.Tracked,
				ProjectRefRemoteKey:      projectRef.Remote,
				ProjectRefRemotePathKey:  projectRef.RemotePath,
				ProjectRefTrackedKey:     projectRef.Tracked,
			},
		},
	)
	return err
}

// ProjectRef returns a string representation of a ProjectRef
func (projectRef *ProjectRef) String() string {
	return projectRef.Identifier
}
