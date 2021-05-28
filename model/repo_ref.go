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
	RepoRefIdKey                      = bsonutil.MustHaveTag(RepoRef{}, "Id")
	RepoRefOwnerKey                   = bsonutil.MustHaveTag(RepoRef{}, "Owner")
	RepoRefRepoKey                    = bsonutil.MustHaveTag(RepoRef{}, "Repo")
	RepoRefEnabledKey                 = bsonutil.MustHaveTag(RepoRef{}, "Enabled")
	RepoRefIdentifierKey              = bsonutil.MustHaveTag(RepoRef{}, "Identifier")
	RepoRefDeactivatePreviousKey      = bsonutil.MustHaveTag(RepoRef{}, "DeactivatePrevious")
	RepoRefHiddenKey                  = bsonutil.MustHaveTag(RepoRef{}, "Hidden")
	RepoRefBatchTimeKey               = bsonutil.MustHaveTag(RepoRef{}, "BatchTime")
	RepoRefFilesIgnoredFromCacheKey   = bsonutil.MustHaveTag(RepoRef{}, "FilesIgnoredFromCache")
	RepoRefDisabledStatsCacheKey      = bsonutil.MustHaveTag(RepoRef{}, "DisabledStatsCache")
	RepoRefGitTagAuthorizedUsersKey   = bsonutil.MustHaveTag(RepoRef{}, "GitTagAuthorizedUsers")
	RepoRefGitTagAuthorizedTeamsKey   = bsonutil.MustHaveTag(RepoRef{}, "GitTagAuthorizedTeams")
	RepoRefTracksPushEventsKey        = bsonutil.MustHaveTag(RepoRef{}, "TracksPushEvents")
	RepoRefDefaultLoggerKey           = bsonutil.MustHaveTag(RepoRef{}, "DefaultLogger")
	RepoRefCedarTestResultsEnabledKey = bsonutil.MustHaveTag(RepoRef{}, "CedarTestResultsEnabled")
	RepoRefPrivateKey                 = bsonutil.MustHaveTag(RepoRef{}, "Private")
	RepoRefRestrictedKey              = bsonutil.MustHaveTag(RepoRef{}, "Restricted")
	RepoRefRemotePathKey              = bsonutil.MustHaveTag(RepoRef{}, "RemotePath")
	RepoRefAdminsKey                  = bsonutil.MustHaveTag(RepoRef{}, "Admins")
	RepoRefPRTestingEnabledKey        = bsonutil.MustHaveTag(RepoRef{}, "PRTestingEnabled")
	RepoRefGithubChecksEnabledKey     = bsonutil.MustHaveTag(RepoRef{}, "GithubChecksEnabled")
	RepoRefGitTagVersionsEnabledKey   = bsonutil.MustHaveTag(RepoRef{}, "GitTagVersionsEnabled")
	RepoRefCommitQueueKey             = bsonutil.MustHaveTag(RepoRef{}, "CommitQueue")
	RepoRefTaskSyncKey                = bsonutil.MustHaveTag(RepoRef{}, "TaskSync")
	RepoRefNotifyOnFailureKey         = bsonutil.MustHaveTag(RepoRef{}, "NotifyOnBuildFailure")
	RepoRefPatchTriggerAliasesKey     = bsonutil.MustHaveTag(RepoRef{}, "PatchTriggerAliases")
	RepoRefPeriodicBuildsKey          = bsonutil.MustHaveTag(RepoRef{}, "PeriodicBuilds")
	RepoRefTriggersKey                = bsonutil.MustHaveTag(RepoRef{}, "Triggers")
	RepoRefWorkstationConfigKey       = bsonutil.MustHaveTag(RepoRef{}, "WorkstationConfig")
	RepoRefRepotrackerDisabledKey     = bsonutil.MustHaveTag(RepoRef{}, "RepotrackerDisabled")
	RepoRefDispatchingDisabledKey     = bsonutil.MustHaveTag(RepoRef{}, "DispatchingDisabled")
	RepoRefPatchingDisabledKey        = bsonutil.MustHaveTag(RepoRef{}, "PatchingDisabled")
	RepoRefSpawnHostScriptPathKey     = bsonutil.MustHaveTag(RepoRef{}, "SpawnHostScriptPath")
)

func (r *RepoRef) Add(creator *user.DBUser) error {
	err := r.Upsert()
	if err != nil {
		return errors.Wrap(err, "Error upserting repo ref")
	}
	return r.AddPermissions(creator)
}

// Insert is included here so ProjectRef.Insert() isn't mistakenly used.
func (r *RepoRef) Insert() error {
	return errors.New("insert not supported for repoRef")
}

// Upsert updates the project ref in the db if an entry already exists,
// overwriting the existing ref. If no project ref exists, one is created
func (r *RepoRef) Upsert() error {
	_, err := db.Upsert(
		RepoRefCollection,
		bson.M{
			RepoRefIdKey: r.Id,
		},
		bson.M{
			"$set": bson.M{
				RepoRefEnabledKey:                 r.Enabled,
				RepoRefPrivateKey:                 r.Private,
				RepoRefRestrictedKey:              r.Restricted,
				RepoRefOwnerKey:                   r.Owner,
				RepoRefRepoKey:                    r.Repo,
				RepoRefRemotePathKey:              r.RemotePath,
				RepoRefAdminsKey:                  r.Admins,
				RepoRefPRTestingEnabledKey:        r.PRTestingEnabled,
				RepoRefPatchingDisabledKey:        r.PatchingDisabled,
				RepoRefRepotrackerDisabledKey:     r.RepotrackerDisabled,
				RepoRefDispatchingDisabledKey:     r.DispatchingDisabled,
				RepoRefSpawnHostScriptPathKey:     r.SpawnHostScriptPath,
				RepoRefIdentifierKey:              r.Identifier,
				RepoRefDeactivatePreviousKey:      r.DeactivatePrevious,
				RepoRefHiddenKey:                  r.Hidden,
				RepoRefBatchTimeKey:               r.BatchTime,
				RepoRefFilesIgnoredFromCacheKey:   r.FilesIgnoredFromCache,
				RepoRefDisabledStatsCacheKey:      r.DisabledStatsCache,
				RepoRefGitTagAuthorizedUsersKey:   r.GitTagAuthorizedUsers,
				RepoRefGitTagAuthorizedTeamsKey:   r.GitTagAuthorizedTeams,
				RepoRefTracksPushEventsKey:        r.TracksPushEvents,
				RepoRefDefaultLoggerKey:           r.DefaultLogger,
				RepoRefCedarTestResultsEnabledKey: r.CedarTestResultsEnabled,
				RepoRefGithubChecksEnabledKey:     r.GithubChecksEnabled,
				RepoRefGitTagVersionsEnabledKey:   r.GitTagVersionsEnabled,
				RepoRefCommitQueueKey:             r.CommitQueue,
				RepoRefTaskSyncKey:                r.TaskSync,
				RepoRefNotifyOnFailureKey:         r.NotifyOnBuildFailure,
				RepoRefPatchTriggerAliasesKey:     r.PatchTriggerAliases,
				RepoRefPeriodicBuildsKey:          r.PeriodicBuilds,
				RepoRefTriggersKey:                r.Triggers,
				RepoRefWorkstationConfigKey:       r.WorkstationConfig,
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

	adminScope := gimlet.Scope{
		ID:        GetRepoAdminScope(r.Id),
		Resources: []string{r.Id},
		Name:      r.Id,
		Type:      evergreen.ProjectResourceType,
	}
	if err := rm.AddScope(adminScope); err != nil {
		return errors.Wrapf(err, "error adding scope for repo project '%s'", r.Id)
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
		return errors.Wrapf(err, "error adding scope for repo project '%s'", r.Id)
	}
	// Create view role for project branch admins
	if !r.IsRestricted() {
		newViewRole := gimlet.Role{
			ID:    GetViewRepoRole(r.Id),
			Scope: adminScope.ID,
			Permissions: gimlet.Permissions{
				evergreen.PermissionProjectSettings: evergreen.ProjectSettingsView.Value,
			},
		}

		if err := rm.UpdateRole(newViewRole); err != nil {
			return errors.Wrapf(err, "error adding view role for repo project '%s'", r.Id)
		}
	}

	if err := rm.UpdateRole(newAdminRole); err != nil {
		return errors.Wrapf(err, "error adding admin role for repo project '%s'", r.Id)
	}
	if creator != nil {
		if err := creator.AddRole(newAdminRole.ID); err != nil {
			return errors.Wrapf(err, "error adding role '%s' to user '%s'", newAdminRole.ID, creator.Id)
		}
	}
	return nil
}

// addViewRepoPermissionsToBranchAdmins gives admins of a project branch permission to view repo ref settings
func addViewRepoPermissionsToBranchAdmins(repoRefID string, admins []string) error {
	catcher := grip.NewBasicCatcher()
	viewRole := GetViewRepoRole(repoRefID)
	for _, admin := range admins {
		newViewer, err := user.FindOneById(admin)
		// ignore errors finding user, since project lists may be outdated
		if err == nil && newViewer != nil {
			if err = newViewer.AddRole(viewRole); err != nil {
				catcher.Wrapf(err, "error adding role '%s' to user '%s'", viewRole, admin)
			}
		}
	}
	return nil
}

// this uses the view repo role specifically, so if the user has this permission through mana then it stays
func removeViewRepoPermissionsFromBranchAdmins(repoRefID string, admins []string) error {
	catcher := grip.NewBasicCatcher()
	viewRole := GetViewRepoRole(repoRefID)
	for _, admin := range admins {
		adminUser, err := user.FindOneById(admin)
		// ignore errors finding user, since project lists may be outdated
		if err == nil && adminUser != nil {
			if err = adminUser.RemoveRole(viewRole); err != nil {
				catcher.Wrapf(err, "error removing role '%s' from user '%s'", viewRole, admin)
			}
		}
	}
	return nil
}

func (r *RepoRef) MakeRestricted(branchProjects []ProjectRef) error {
	rm := evergreen.GetEnvironment().RoleManager()
	scopeId := GetUnrestrictedBranchProjectsScope(r.Id)
	branchOnlyAdmins := []string{}
	// if the branch project is now restricted, remove it from the unrestricted scope
	for _, p := range branchProjects {
		if !p.IsRestricted() {
			continue
		}
		if err := rm.RemoveResourceFromScope(scopeId, p.Id); err != nil {
			return errors.Wrapf(err, "error removing resource '%s' from unrestricted branches scope", p.Id)
		}
		if err := rm.RemoveResourceFromScope(evergreen.UnrestrictedProjectsScope, p.Id); err != nil {
			return errors.Wrapf(err, "error removing resource '%s' from unrestricted projects scope")
		}
		if err := rm.AddResourceToScope(evergreen.RestrictedProjectsScope, p.Id); err != nil {
			return errors.Wrapf(err, "error adding resource '%s' to restricted projects scope")
		}

		// get branch admins that aren't repo admins and remove view repo permissions
		_, adminsToModify := utility.StringSliceSymmetricDifference(r.Admins, p.Admins)
		branchOnlyAdmins = append(branchOnlyAdmins, adminsToModify...)
	}
	branchOnlyAdmins = utility.UniqueStrings(branchOnlyAdmins)
	return removeViewRepoPermissionsFromBranchAdmins(r.Id, branchOnlyAdmins)
}

func (r *RepoRef) MakeUnrestricted(branchProjects []ProjectRef) error {
	rm := evergreen.GetEnvironment().RoleManager()
	scopeId := GetUnrestrictedBranchProjectsScope(r.Id)
	branchOnlyAdmins := []string{}
	// if the branch project is now unrestricted, add it to the unrestricted scopes
	for _, p := range branchProjects {
		if p.IsRestricted() {
			continue
		}
		if err := rm.AddResourceToScope(scopeId, p.Id); err != nil {
			return errors.Wrapf(err, "error adding resource '%s' to unrestricted branches scope", p.Id)
		}
		if err := rm.RemoveResourceFromScope(evergreen.RestrictedProjectsScope, p.Id); err != nil {
			return errors.Wrapf(err, "error removing resource '%s' from restricted projects scope")
		}
		if err := rm.AddResourceToScope(evergreen.UnrestrictedProjectsScope, p.Id); err != nil {
			return errors.Wrapf(err, "error adding resource '%s' to unrestricted projects scope")
		}
		// get branch admins that aren't repo admins and remove add repo permissions
		_, adminsToModify := utility.StringSliceSymmetricDifference(r.Admins, p.Admins)
		branchOnlyAdmins = append(branchOnlyAdmins, adminsToModify...)
	}
	branchOnlyAdmins = utility.UniqueStrings(branchOnlyAdmins)
	return addViewRepoPermissionsToBranchAdmins(r.Id, branchOnlyAdmins)
}

func (r *RepoRef) UpdateAdminRoles(toAdd, toRemove []string) error {
	if len(toAdd) == 0 && len(toRemove) == 0 {
		return nil
	}

	adminRole := GetRepoAdminRole(r.Id)
	for _, addedUser := range toAdd {
		adminUser, err := user.FindOneById(addedUser)
		if err != nil {
			return errors.Wrapf(err, "error finding user '%s'", addedUser)
		}
		if adminUser == nil {
			return errors.Errorf("no user '%s' found", addedUser)
		}
		if err = adminUser.AddRole(adminRole); err != nil {
			return errors.Wrapf(err, "error adding role %s to user %s", adminRole, addedUser)
		}

	}
	for _, removedUser := range toRemove {
		adminUser, err := user.FindOneById(removedUser)
		if err != nil {
			return errors.Wrapf(err, "error finding user %s", removedUser)
		}
		if adminUser == nil {
			continue
		}

		if err = adminUser.RemoveRole(adminRole); err != nil {
			return errors.Wrapf(err, "error removing role %s from user %s", adminRole, removedUser)
		}
	}
	return nil
}

func addAdminToRepo(repoId, admin string) error {
	return db.UpdateId(
		RepoRefCollection,
		repoId,
		bson.M{
			"$push": bson.M{RepoRefAdminsKey: admin},
		},
	)
}

// GetUnrestrictedBranchProjectsScope returns the scope ID that includes the unrestricted branches for this project.
func GetUnrestrictedBranchProjectsScope(repoId string) string {
	return fmt.Sprintf("unrestricted_branches_%s", repoId)
}

// GetRepoAdminScope returns the scope ID that includes all branch projects, regardless of restricted/unrestricted.
func GetRepoAdminScope(repoId string) string {
	return fmt.Sprintf("admin_repo_%s", repoId)
}

func GetRepoAdminRole(repoId string) string {
	return fmt.Sprintf("admin_repo_%s", repoId)
}

func GetViewRepoRole(repoId string) string {
	return fmt.Sprintf("view_repo_%s", repoId)
}
