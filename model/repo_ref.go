package model

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
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
	RepoRefIdKey             = bsonutil.MustHaveTag(RepoRef{}, "Id")
	RepoRefOwnerKey          = bsonutil.MustHaveTag(RepoRef{}, "Owner")
	RepoRefRepoKey           = bsonutil.MustHaveTag(RepoRef{}, "Repo")
	RepoRefAdminsKey         = bsonutil.MustHaveTag(RepoRef{}, "Admins")
	RepoRefCommitQueueKey    = bsonutil.MustHaveTag(RepoRef{}, "CommitQueue")
	RepoRefPeriodicBuildsKey = bsonutil.MustHaveTag(RepoRef{}, "PeriodicBuilds")
	RepoRefTriggersKey       = bsonutil.MustHaveTag(RepoRef{}, "Triggers")
)

func (r *RepoRef) Add(creator *user.DBUser) error {
	if err := r.Upsert(); err != nil {
		return errors.Wrap(err, "upserting repo ref")
	}
	return r.addPermissions(creator)
}

// Insert is included here so ProjectRef.Insert() isn't mistakenly used.
func (r *RepoRef) Insert() error {
	return errors.New("insert not supported for repoRef")
}

// Upsert updates the project ref in the db if an entry already exists,
// overwriting the existing ref. If no project ref exists, one is created.
// Ensures that fields that aren't relevant to repos aren't set.
func (r *RepoRef) Upsert() error {
	r.RepoRefId = ""
	r.Branch = ""
	_, err := db.Upsert(
		RepoRefCollection,
		bson.M{
			RepoRefIdKey: r.Id,
		},
		r,
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

// FindRepoRefsToSync finds all repo refs that have Parameter Sore enabled but
// don't have their project variables synced to Parameter Store yet.
// TODO (DEVPROD-11882): remove this function once the rollout is stable.
func FindRepoRefsToSync(ctx context.Context) ([]RepoRef, error) {
	cur, err := evergreen.GetEnvironment().DB().Collection(RepoRefCollection).Find(ctx, psEnabledButNotSyncedQuery)
	if err != nil {
		return nil, errors.Wrap(err, "finding repo refs to sync")
	}
	repoRefs := []RepoRef{}
	if err := cur.All(ctx, &repoRefs); err != nil {
		return nil, errors.Wrap(err, "decoding repo refs to sync")
	}
	return repoRefs, nil
}

// addPermissions adds the repo ref to the general scope and gives the inputted creator admin permissions
// for the repo and branches.
func (r *RepoRef) addPermissions(creator *user.DBUser) error {
	rm := evergreen.GetEnvironment().RoleManager()
	// Add to the general scope.
	if err := rm.AddResourceToScope(evergreen.AllProjectsScope, r.Id); err != nil {
		return errors.Wrapf(err, "adding repo '%s' to the scope '%s'", r.Id, evergreen.AllProjectsScope)
	}

	adminScope := gimlet.Scope{
		ID:        GetRepoAdminScope(r.Id),
		Resources: []string{r.Id}, // projects that use repo settings will also be added to this scope
		Name:      r.Id,
		Type:      evergreen.ProjectResourceType,
	}
	if err := rm.AddScope(adminScope); err != nil {
		return errors.Wrapf(err, "adding scope for repo project '%s'", r.Id)
	}

	// will be used to request permissions at the repo level
	unrestrictedBranchesScope := gimlet.Scope{
		ID:        GetUnrestrictedBranchProjectsScope(r.Id),
		Resources: []string{}, // will add resources by project
		Type:      evergreen.ProjectResourceType,
	}
	newAdminRole := gimlet.Role{
		ID:          GetRepoAdminRole(r.Id),
		Scope:       adminScope.ID,
		Permissions: adminPermissions,
	}
	if err := rm.AddScope(unrestrictedBranchesScope); err != nil {
		return errors.Wrapf(err, "adding scope for repo project '%s'", r.Id)
	}

	if err := rm.UpdateRole(newAdminRole); err != nil {
		return errors.Wrapf(err, "adding admin role for repo project '%s'", r.Id)
	}
	if creator != nil {
		if err := creator.AddRole(newAdminRole.ID); err != nil {
			return errors.Wrapf(err, "adding role '%s' to user '%s'", newAdminRole.ID, creator.Id)
		}
	}
	return nil
}

func (r *RepoRef) MakeRestricted(branchProjects []ProjectRef) error {
	rm := evergreen.GetEnvironment().RoleManager()
	scopeId := GetUnrestrictedBranchProjectsScope(r.Id)

	// if the branch project is now restricted, remove it from the unrestricted scope
	for _, p := range branchProjects {
		if !p.IsRestricted() {
			continue
		}
		if err := rm.RemoveResourceFromScope(scopeId, p.Id); err != nil {
			return errors.Wrapf(err, "removing resource '%s' from unrestricted branches scope", p.Id)
		}
		if err := rm.RemoveResourceFromScope(evergreen.UnrestrictedProjectsScope, p.Id); err != nil {
			return errors.Wrapf(err, "removing resource '%s' from unrestricted projects scope", p.Id)
		}
		if err := rm.AddResourceToScope(evergreen.RestrictedProjectsScope, p.Id); err != nil {
			return errors.Wrapf(err, "adding resource '%s' to restricted projects scope", p.Id)
		}
	}
	return nil
}

func (r *RepoRef) MakeUnrestricted(branchProjects []ProjectRef) error {
	rm := evergreen.GetEnvironment().RoleManager()
	scopeId := GetUnrestrictedBranchProjectsScope(r.Id)
	// if the branch project is now unrestricted, add it to the unrestricted scopes
	for _, p := range branchProjects {
		if p.IsRestricted() {
			continue
		}
		if err := rm.AddResourceToScope(scopeId, p.Id); err != nil {
			return errors.Wrapf(err, "adding resource '%s' to unrestricted branches scope", p.Id)
		}
		if err := rm.RemoveResourceFromScope(evergreen.RestrictedProjectsScope, p.Id); err != nil {
			return errors.Wrapf(err, "removing resource '%s' from restricted projects scope", p.Id)
		}
		if err := rm.AddResourceToScope(evergreen.UnrestrictedProjectsScope, p.Id); err != nil {
			return errors.Wrapf(err, "adding resource '%s' to unrestricted projects scope", p.Id)
		}
	}
	return nil
}

func (r *RepoRef) UpdateAdminRoles(toAdd, toRemove []string) error {
	if len(toAdd) == 0 && len(toRemove) == 0 {
		return nil
	}

	catcher := grip.NewBasicCatcher()
	adminRole := GetRepoAdminRole(r.Id)
	for _, addedUser := range toAdd {
		adminUser, err := user.FindOneById(addedUser)
		if err != nil {
			catcher.Wrapf(err, "finding user '%s'", addedUser)
			r.removeFromAdminsList(addedUser)
			continue
		}
		if adminUser == nil {
			catcher.Errorf("no user '%s' found", addedUser)
			r.removeFromAdminsList(addedUser)
			continue
		}
		if err = adminUser.AddRole(adminRole); err != nil {
			catcher.Wrapf(err, "adding role '%s' to user '%s'", adminRole, addedUser)
			r.removeFromAdminsList(addedUser)
			continue
		}

	}
	for _, removedUser := range toRemove {
		adminUser, err := user.FindOneById(removedUser)
		if err != nil {
			catcher.Wrapf(err, "finding user '%s'", removedUser)
			continue
		}
		if adminUser == nil {
			continue
		}

		if err = adminUser.RemoveRole(adminRole); err != nil {
			catcher.Wrapf(err, "removing role '%s' from user '%s'", adminRole, removedUser)
			r.Admins = append(r.Admins, removedUser)
			continue
		}
	}
	if err := catcher.Resolve(); err != nil {
		return errors.Wrap(err, "updating some admins")
	}
	return nil
}

// GetUnrestrictedBranchProjectsScope returns the scope ID that includes the unrestricted branches for this project.
func GetUnrestrictedBranchProjectsScope(repoId string) string {
	return fmt.Sprintf("unrestricted_branches_%s", repoId)
}

// GetRepoAdminScope returns the scope ID that includes all branch projects, regardless of restricted/unrestricted.
func GetRepoAdminScope(repoId string) string {
	return fmt.Sprintf("admin_repo_%s", repoId)
}

// GetRepoAdminRole returns the repo admin role ID for the given repo.
func GetRepoAdminRole(repoId string) string {
	return fmt.Sprintf("admin_repo_%s", repoId)
}
