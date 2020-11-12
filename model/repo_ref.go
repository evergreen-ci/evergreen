package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
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

// findOneRepoRefQ returns one RepoRef that satisfies the query.
func findOneRepoRefQ(query db.Q) (*RepoRef, error) {
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
	return findOneRepoRefQ(db.Query(bson.M{
		RepoRefIdKey: identifier,
	}))
}

// FindRepoRefsByRepoAndBranch finds RepoRefs with matching repo/branch
// that are enabled and setup for PR testing
func FindRepoRefByOwnerAndRepo(owner, repoName string) (*RepoRef, error) {
	return findOneRepoRefQ(db.Query(bson.M{
		RepoRefOwnerKey: owner,
		RepoRefRepoKey:  repoName,
	}))
}

func (r *RepoRef) AddPermissions(creator *user.DBUser) error {
	rm := evergreen.GetEnvironment().RoleManager()
	catcher := grip.NewBasicCatcher()
	if !r.Restricted {
		catcher.Wrapf(rm.AddResourceToScope(evergreen.UnrestrictedProjectsScope, r.Id), "error adding repo project '%s' to list of unrestricted projects", r.Id)
	}
	catcher.Wrapf(rm.AddResourceToScope(evergreen.AllProjectsScope, r.Id), "error adding repo project '%s' to list of all projects", r.Id)
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	newScope := gimlet.Scope{
		ID:          fmt.Sprintf("repo_%s", r.Id),
		Resources:   []string{r.Id},
		Name:        r.Id,
		Type:        evergreen.ProjectResourceType,
		ParentScope: evergreen.AllProjectsScope, // does this seem reasonable or will it result in duplicates?
	}
	if err := rm.AddScope(newScope); err != nil {
		return errors.Wrapf(err, "error adding scope for repo project '%s'", r.Id)
	}

	newRole := gimlet.Role{
		ID:          fmt.Sprintf("admin_repo_%s", r.Id),
		Scope:       newScope.ID,
		Permissions: adminPermissions,
	}
	if creator != nil {
		newRole.Owners = []string{creator.Id}
	}
	if err := rm.UpdateRole(newRole); err != nil {
		return errors.Wrapf(err, "error adding admin role for repo project '%s'", r.Id)
	}
	if creator != nil {
		if err := creator.AddRole(newRole.ID); err != nil {
			return errors.Wrapf(err, "error adding role '%s' to user '%s'", newRole.ID, creator.Id)
		}
	}
	return nil
}

func (r *RepoRef) HasEditPermission(u *user.DBUser) bool {
	if utility.StringSliceContains(r.Admins, u.Username()) {
		return true
	}
	return u.HasPermission(gimlet.PermissionOpts{
		Resource:      r.Id,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsEdit.Value,
	})
}
