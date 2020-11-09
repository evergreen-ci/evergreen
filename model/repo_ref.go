package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

const RepoRefCollection = "repo_ref"

// RepoRef is a wrapper for ProjectRef, as many settings in the project ref
// can be defined at both the branch and repo level.
type RepoRef struct {
	ProjectRef `yaml:",inline" bson:",inline"`
}

var (
	// bson fields for the RepoRef struct
	RepoRefIdKey                  = bsonutil.MustHaveTag(RepoRef{}, "Id")
	RepoRefOwnerKey               = bsonutil.MustHaveTag(RepoRef{}, "Owner")
	RepoRefRepoKey                = bsonutil.MustHaveTag(RepoRef{}, "Repo")
	RepoRefEnabledKey             = bsonutil.MustHaveTag(RepoRef{}, "Enabled")
	RepoRefPrivateKey             = bsonutil.MustHaveTag(RepoRef{}, "Private")
	RepoRefRestrictedKey          = bsonutil.MustHaveTag(RepoRef{}, "Restricted")
	RepoRefDisplayNameKey         = bsonutil.MustHaveTag(RepoRef{}, "DisplayName")
	RepoRefRemotePathKey          = bsonutil.MustHaveTag(RepoRef{}, "RemotePath")
	RepoRefAdminsKey              = bsonutil.MustHaveTag(RepoRef{}, "Admins")
	RepoRefPRTestingEnabledKey    = bsonutil.MustHaveTag(RepoRef{}, "PRTestingEnabled")
	RepoRefRepotrackerDisabledKey = bsonutil.MustHaveTag(RepoRef{}, "RepotrackerDisabled")
	RepoRefDispatchingDisabledKey = bsonutil.MustHaveTag(RepoRef{}, "DispatchingDisabled")
	RepoRefPatchingDisabledKey    = bsonutil.MustHaveTag(RepoRef{}, "PatchingDisabled")
	RepoRefSpawnHostScriptPathKey = bsonutil.MustHaveTag(RepoRef{}, "SpawnHostScriptPath")
)

func (RepoRef *RepoRef) Insert() error {
	return db.Insert(RepoRefCollection, RepoRef)
}

func (RepoRef *RepoRef) Update() error {
	return db.Update(
		RepoRefCollection,
		bson.M{
			RepoRefIdKey: RepoRef.Id,
		},
		RepoRef,
	)
}

// Upsert updates the project ref in the db if an entry already exists,
// overwriting the existing ref. If no project ref exists, one is created
func (RepoRef *RepoRef) Upsert() error {
	_, err := db.Upsert(
		RepoRefCollection,
		bson.M{
			RepoRefIdKey: RepoRef.Id,
		},
		bson.M{
			"$set": bson.M{
				RepoRefEnabledKey:             RepoRef.Enabled,
				RepoRefPrivateKey:             RepoRef.Private,
				RepoRefRestrictedKey:          RepoRef.Restricted,
				RepoRefOwnerKey:               RepoRef.Owner,
				RepoRefRepoKey:                RepoRef.Repo,
				RepoRefDisplayNameKey:         RepoRef.DisplayName,
				RepoRefRemotePathKey:          RepoRef.RemotePath,
				RepoRefAdminsKey:              RepoRef.Admins,
				RepoRefPRTestingEnabledKey:    RepoRef.PRTestingEnabled,
				RepoRefPatchingDisabledKey:    RepoRef.PatchingDisabled,
				RepoRefRepotrackerDisabledKey: RepoRef.RepotrackerDisabled,
				RepoRefDispatchingDisabledKey: RepoRef.DispatchingDisabled,
				RepoRefSpawnHostScriptPathKey: RepoRef.SpawnHostScriptPath,
			},
		},
	)
	return err
}

// FindOne returns one RepoRef that satisfies the query.
func FindOne(query db.Q) (*RepoRef, error) {
	repoRef := &RepoRef{}
	err := db.FindOneQ(RepoRefCollection, query, repoRef)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return repoRef, err
}

// FindOneRepoRef gets a project ref given the owner name, the repo
// name and the project name
func FindOneRepoRef(identifier string) (*RepoRef, error) {
	return FindOne(db.Query(bson.M{
		RepoRefIdKey: identifier,
	}))
}

// FindRepoRefsByRepoAndBranch finds RepoRefs with matching repo/branch
// that are enabled and setup for PR testing
func FindRepoRefByOwnerAndRepo(owner, repoName string) (*RepoRef, error) {
	return FindOne(db.Query(bson.M{
		RepoRefOwnerKey: owner,
		RepoRefRepoKey:  repoName,
	}))
}
