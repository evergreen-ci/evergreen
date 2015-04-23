package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"gopkg.in/yaml.v1"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"time"
)

const (
	VersionsCollection = "versions"
)

// MCIVersion stores information pertaining to the MCI app
type MCIVersion struct {
	Name    string  `bson:"name"`
	Version float64 `bson:"version"`
}

// BuildStatus stores metadata relating to each build
type BuildStatus struct {
	BuildVariant string    `bson:"build_variant" json:"id"`
	Activated    bool      `bson:"activated" json:"activated"`
	ActivateAt   time.Time `bson:"activate_at,omitempty" json:"activate_at,omitempty"`
	BuildId      string    `bson:"build_id,omitempty" json:"build_id,omitempty"`
}

var (
	BuildStatusVariantKey    = MustHaveBsonTag(BuildStatus{}, "BuildVariant")
	BuildStatusActivatedKey  = MustHaveBsonTag(BuildStatus{}, "Activated")
	BuildStatusActivateAtKey = MustHaveBsonTag(BuildStatus{}, "ActivateAt")
	BuildStatusBuildIdKey    = MustHaveBsonTag(BuildStatus{}, "BuildId")
)

type Version struct {
	Id                  string        `bson:"_id" json:"id,omitempty"`
	CreateTime          time.Time     `bson:"create_time" json:"create_time,omitempty"`
	StartTime           time.Time     `bson:"start_time" json:"start_time,omitempty"`
	FinishTime          time.Time     `bson:"finish_time" json:"finish_time,omitempty"`
	Project             string        `bson:"branch" json:"branch,omitempty"`
	Revision            string        `bson:"gitspec" json:"revision,omitempty"`
	Author              string        `bson:"author" json:"author,omitempty"`
	AuthorEmail         string        `bson:"author_email" json:"author_email,omitempty"`
	Message             string        `bson:"message" json:"message,omitempty"`
	Status              string        `bson:"status" json:"status,omitempty"`
	RevisionOrderNumber int           `bson:"order,omitempty" json:"order,omitempty"`
	Config              string        `bson:"config" json:"config,omitempty"`
	Owner               string        `bson:"owner_name" json:"owner_name,omitempty"`
	Repo                string        `bson:"repo_name" json:"repo_name,omitempty"`
	Branch              string        `bson:"branch_name" json:"branch_name,omitempty"`
	RepoKind            string        `bson:"repo_kind" json:"repo_kind,omitempty"`
	BuildVariants       []BuildStatus `bson:"build_variants_status,omitempty" json:"build_variants_status,omitempty"`

	// This is technically redundant, but a lot of code relies on it, so I'm going to leave it
	BuildIds []string `bson:"builds" json:"builds,omitempty"`

	Identifier string `bson:"identifier" json:"identifier,omitempty"`
	Remote     bool   `bson:"remote" json:"remote,omitempty"`
	RemotePath string `bson:"remote_path" json:"remote_path,omitempty"`
	// version requester - this is used to help tell the
	// reason this version was created. e.g. it could be
	// because the repotracker requested it (via tracking the
	// repository) or it was triggered by a developer
	// patch request
	Requester string `bson:"r" json:"requester,omitempty"`
	// version errors - this is used to keep track of any errors that were
	// encountered in the process of creating a version. If there are no errors
	// this field is omitted in the database
	Errors []string `bson:"errors,omitempty" json:"errors,omitempty"`
}

var (
	// bson fields for the version struct
	VersionIdKey                  = MustHaveBsonTag(Version{}, "Id")
	VersionCreateTimeKey          = MustHaveBsonTag(Version{}, "CreateTime")
	VersionStartTimeKey           = MustHaveBsonTag(Version{}, "StartTime")
	VersionFinishTimeKey          = MustHaveBsonTag(Version{}, "FinishTime")
	VersionProjectKey             = MustHaveBsonTag(Version{}, "Project")
	VersionRevisionKey            = MustHaveBsonTag(Version{}, "Revision")
	VersionAuthorKey              = MustHaveBsonTag(Version{}, "Author")
	VersionAuthorEmailKey         = MustHaveBsonTag(Version{}, "AuthorEmail")
	VersionMessageKey             = MustHaveBsonTag(Version{}, "Message")
	VersionStatusKey              = MustHaveBsonTag(Version{}, "Status")
	VersionBuildIdsKey            = MustHaveBsonTag(Version{}, "BuildIds")
	VersionBuildVariantsKey       = MustHaveBsonTag(Version{}, "BuildVariants")
	VersionRevisionOrderNumberKey = MustHaveBsonTag(Version{}, "RevisionOrderNumber")
	VersionRequesterKey           = MustHaveBsonTag(Version{}, "Requester")
	VersionConfigKey              = MustHaveBsonTag(Version{}, "Config")
	VersionOwnerNameKey           = MustHaveBsonTag(Version{}, "Owner")
	VersionRepoKey                = MustHaveBsonTag(Version{}, "Repo")
	VersionProjectNameKey         = MustHaveBsonTag(Version{}, "Branch")
	VersionRepoKindKey            = MustHaveBsonTag(Version{}, "RepoKind")
	VersionErrorsKey              = MustHaveBsonTag(Version{}, "Errors")
	VersionIdentifierKey          = MustHaveBsonTag(Version{}, "Identifier")
	VersionRemoteKey              = MustHaveBsonTag(Version{}, "Remote")
	VersionRemoteURLKey           = MustHaveBsonTag(Version{}, "RemotePath")
)

func FindOneVersion(query interface{}, projection interface{}) (*Version, error) {
	version := &Version{}
	err := db.FindOne(
		VersionsCollection,
		query,
		projection,
		db.NoSort,
		version,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return version, err
}

// finds all versions matching the specified interface
func FindAllVersions(query interface{}, projection interface{}, sort []string,
	skip int, limit int) ([]Version, error) {
	versions := []Version{}
	err := db.FindAll(
		VersionsCollection,
		query,
		projection,
		sort,
		skip,
		limit,
		&versions,
	)
	return versions, err
}

func FindVersion(id string) (*Version, error) {
	return FindOneVersion(
		bson.M{
			VersionIdKey: id,
		},
		db.NoProjection,
	)
}

func LastKnownGoodConfig(identifier string) ([]Version, error) {
	return FindAllVersions(
		bson.M{
			VersionIdentifierKey: identifier,
			VersionRequesterKey:  mci.RepotrackerVersionRequester,
			VersionErrorsKey: bson.M{
				"$exists": false,
			},
		},
		db.NoProjection,
		[]string{"-" + VersionRevisionOrderNumberKey},
		db.NoSkip,
		db.NoLimit,
	)
}

func FindVersionByIdAndRevision(project, revision string) (*Version, error) {
	return FindOneVersion(
		bson.M{
			VersionProjectKey:   project,
			VersionRevisionKey:  revision,
			VersionRequesterKey: mci.RepotrackerVersionRequester,
		},
		db.NoProjection,
	)
}

// finds just the Id string of a base version from a given gitspec,
// returns empty string if nothing is found.
func FindBaseVersionIdForRevision(project, revision string) string {
	version, err := FindVersionByIdAndRevision(project, revision)
	if err != nil || version == nil {
		return ""
	}
	return version.Id
}

// Finds the most recent version for this project
func FindMostRecentVersion(projectId string) (*Version, error) {
	// version is an array containing one element
	version, err := FindMostRecentVersions(projectId, mci.RepotrackerVersionRequester, 0, 1)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &version[0], nil
}

func FindMostRecentVersions(projectId string, requester string, skip int,
	limit int) ([]Version, error) {
	return FindAllVersions(
		bson.M{
			VersionRequesterKey: requester,
			VersionProjectKey:   projectId,
		},
		db.NoProjection,
		[]string{"-" + VersionRevisionOrderNumberKey},
		skip,
		limit,
	)
}

func TotalVersions(query interface{}) (int, error) {
	return db.Count(VersionsCollection, query)
}

func (self *Version) SetAllTasksAborted(aborted bool) error {
	_, err := UpdateAllTasks(
		bson.M{
			TaskBuildIdKey: bson.M{
				"$in": self.BuildIds,
			},
			TaskStatusKey: bson.M{
				"$in": mci.AbortableStatuses,
			},
		},
		bson.M{
			"$set": bson.M{
				TaskAbortedKey: aborted,
			},
		},
	)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error marking all tasks for builds"+
			" in version %v as aborted=%v", self.Id, aborted)
		return err
	}

	return nil
}

// TryMarkVersionStarted attempts to mark a version as started if it
// isn't already marked as such
func TryMarkVersionStarted(versionId string, startTime time.Time) error {
	filter := bson.M{
		VersionIdKey:     versionId,
		VersionStatusKey: mci.VersionCreated,
	}
	update := bson.M{
		"$set": bson.M{
			VersionStartTimeKey: startTime,
			VersionStatusKey:    mci.VersionStarted,
		},
	}
	err := UpdateOneVersion(filter, update)
	if err == mgo.ErrNotFound {
		return nil
	}
	return err
}

func (self *Version) UpdateBuildVariants() error {
	return UpdateOneVersion(
		bson.M{
			VersionIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				VersionBuildVariantsKey: self.BuildVariants,
			},
		},
	)
}

func (self *Version) SetActivated(active bool) error {
	// mark locally
	now := time.Now()
	for variant, status := range self.BuildVariants {
		status.Activated = active
		if active {
			status.ActivateAt = now
		}
		self.BuildVariants[variant] = status
	}

	err := self.UpdateBuildVariants()

	if err != nil {
		return err
	}

	// set all the builds
	for _, buildStatus := range self.BuildVariants {
		build, err := FindBuild(buildStatus.BuildId)
		if err != nil {
			return err
		} else if build == nil {
			mci.Logger.Logf(slogger.WARN, "Build '%v' was not found", buildStatus.BuildId)
			continue
		}
		if err := build.SetActivated(active, false); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error marking build %v as"+
				" activated=%v", buildStatus.BuildId, active)
			return err
		}
	}
	// success
	return nil
}

func (self *Version) SetPriority(priority int) error {
	//set version and all its builds to active
	if err := self.SetActivated(true); err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error marking version %v as"+
			" active=true", self.Id)
		return err
	}

	// set all the builds to proper priority
	for _, buildId := range self.BuildIds {
		build, err := FindBuild(buildId)
		if err != nil {
			return err
		}
		if err := build.SetPriority(priority); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error marking build %v as"+
				" priority=%v", buildId, priority)
			return err
		}
	}

	// success
	return nil
}

func SetPatchVersionActivated(versionId string, active bool) error {
	version, err := FindVersion(versionId)

	if err != nil {
		return err
	}

	if version == nil {
		return fmt.Errorf("version %v does not exist", versionId)
	}

	// activate all of the builds for the version
	for _, buildId := range version.BuildIds {
		build, err := FindBuild(buildId)
		if err != nil {
			return err
		}
		err = build.SetActivated(active, false)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error activating build %v: %v",
				buildId, err)
		}
	}

	if active {
		return version.SetActivated(active)
	} else {
		return nil
	}
}

func (self *Version) Insert() error {
	return db.Insert(VersionsCollection, self)
}

func UpdateOneVersion(query interface{}, update interface{}) error {
	return db.Update(
		VersionsCollection,
		query,
		update,
	)
}

// revisionToVersion combines information from both the repository data and
// the project ref to create a version document
func revisionToVersion(revision *Revision,
	projectRef *ProjectRef) *Version {
	return &Version{
		Author:      revision.Author,
		AuthorEmail: revision.AuthorEmail,
		Branch:      projectRef.Branch,
		CreateTime:  revision.CreateTime,
		Id:          makeVersionId(projectRef, revision),
		Identifier:  projectRef.Identifier,
		Message:     revision.RevisionMessage,
		Owner:       projectRef.Owner,
		Project:     projectRef.String(),
		Remote:      projectRef.Remote,
		RemotePath:  projectRef.RemotePath,
		Repo:        projectRef.Repo,
		RepoKind:    projectRef.RepoKind,
		Requester:   mci.RepotrackerVersionRequester,
		Revision:    revision.Revision,
		Status:      mci.VersionCreated,
	}
}

// constructs a version id from the project name and git revision
func makeVersionId(projectRef *ProjectRef, revision *Revision) string {
	return CleanName(fmt.Sprintf("%v_%v", projectRef.String(), revision.Revision))
}

// Returns the version corresponding to a given revision of a project
func FindVersionByProjectAndRevision(projectRef *ProjectRef, revision *Revision) (*Version, error) {
	v, err := FindVersion(makeVersionId(projectRef, revision))
	if err != nil {
		return nil, err
	}
	return v, nil
}

// CreateStubVersionFromRevision creates a version in the database
// containing information from the given repository revision object
// (revision) and some information (errs) explaining why a full
// version (along with builds/tasks) is not being created.
// Returns the created version.
func CreateStubVersionFromRevision(projectRef *ProjectRef,
	revision *Revision, mciSettings *mci.MCISettings, errs []string) (
	version *Version, err error) {
	version = revisionToVersion(revision, projectRef)
	version.Errors = errs
	return createProjectVersion(projectRef, &Project{}, version, mciSettings)
}

// CreateVersionFromRevision creates a version in the database containing
// information from the given repository revision (revision) and project
// configuration (projectConfig).
// Note: this method will trigger a DuplicateKeyError if the version already exists
// Returns the created version.
func CreateVersionFromRevision(
	projectRef *ProjectRef, projectConfig *Project,
	revision *Revision, mciSettings *mci.MCISettings) (version *Version, err error) {
	// marshall the project YAML for storage
	projectYamlBytes, err := yaml.Marshal(projectConfig)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling project config from repository "+
			"revision “%v”: %v", revision.Revision, err)
	}
	version = revisionToVersion(revision, projectRef)
	version.Config = string(projectYamlBytes)
	return createProjectVersion(projectRef, projectConfig, version, mciSettings)
}

// Retrieves the last activation time of this build variant. If the build variant has never
// been activated we return nil.
func variantLastActivationTime(projectRef *ProjectRef, variant *BuildVariant) (*time.Time, error) {
	projectId := projectRef.String()
	versions, err := FindAllVersions(
		bson.M{
			VersionProjectKey:   projectId,
			VersionRequesterKey: mci.RepotrackerVersionRequester,
			VersionBuildVariantsKey: bson.M{
				"$elemMatch": bson.M{
					BuildStatusActivatedKey: true,
					BuildStatusVariantKey:   variant.Name,
				},
			},
		},
		bson.M{
			VersionBuildVariantsKey: 1,
		},
		[]string{"-" + VersionRevisionOrderNumberKey},
		db.NoSkip,
		1,
	)
	if err != nil {
		return nil, err
	}

	if versions == nil || len(versions) < 1 {
		// We don't have an activation time for this variant.
		// just use the current time
		// this might be a new variant
		mci.Logger.Logf(slogger.INFO, "Don't have past activation time for variant %v"+
			" project %v, using the current time so it gets activated now",
			variant.Name, projectId)
		return nil, nil
	}

	if len(versions) > 1 {
		panic(fmt.Sprintf("MongoDB fatal error: while querying for previously activated versions"+
			" for project %v, buildvariant %v, we limited to 1 version, but got %v instead",
			projectId, variant.Name, len(versions)))
	}

	for _, buildStatus := range versions[0].BuildVariants {
		if buildStatus.BuildVariant == variant.Name && buildStatus.Activated {
			return &buildStatus.ActivateAt, nil
		}
	}

	// If MongoDB returns a doc for this query, the above loop should actually find
	// a matching buildVariant
	// So this line should never be reached
	panic(fmt.Sprintf("MongoDB fatal error: while querying for previously activated versions"+
		" for project %v, buildvariant %v, got version doc with id %v that doesn't match query",
		projectId, variant.Name, versions[0].Id))
}

// createProjectVersion creates and inserts the necessary documents pertaining
// to a given project ref and version - these include the accompanying builds
// and tasks that are part of the version document
// Returns the created version
func createProjectVersion(projectRef *ProjectRef, project *Project,
	version *Version, mciSettings *mci.MCISettings) (*Version, error) {
	projectId := projectRef.String()
	revisionOrderNumber, err := GetNewRevisionOrderNumber(projectId)
	if err != nil {
		return nil, fmt.Errorf("Error getting new commit order number: %v",
			err.Error())
	}

	versions, err := FindAllVersions(
		bson.M{
			VersionRequesterKey: mci.RepotrackerVersionRequester,
			VersionProjectKey:   projectId,
		},
		bson.M{
			VersionRevisionOrderNumberKey: 1,
		},
		[]string{"-" + VersionRevisionOrderNumberKey},
		db.NoSkip,
		1,
	)
	if err != nil {
		return nil, fmt.Errorf("Error getting latest version: %v", err.Error())
	}

	if len(versions) != 1 {
		mci.Logger.Logf(slogger.WARN, "Unable to locate any version in the"+
			" database. Skipping sanity check")
	} else {
		if revisionOrderNumber <= versions[0].RevisionOrderNumber {
			panic(fmt.Sprintf("Retrieved commit order number isn't strictly"+
				" greater than latest version commit order number: %v <= %v",
				revisionOrderNumber, versions[0].RevisionOrderNumber))
		}
	}
	version.RevisionOrderNumber = revisionOrderNumber

	for _, buildvariant := range project.BuildVariants {
		if buildvariant.Disabled {
			continue
		}
		buildId, err := CreateBuildFromVersion(project, version,
			buildvariant.Name, false, nil, mciSettings)
		if err != nil {
			return nil, err
		}

		lastActivation, err := variantLastActivationTime(projectRef, &buildvariant)
		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "Error getting activation time for "+
				"bv %v project %v", buildvariant.Name, projectRef.String())
			return nil, err
		}

		var activateAt time.Time
		if lastActivation == nil {
			// if we don't have a last activation time then activate now.
			activateAt = time.Now()
		} else {
			activateAt = lastActivation.Add(time.Minute * time.Duration(project.GetBatchTime(&buildvariant)))
			mci.Logger.Logf(slogger.INFO, "Going to activate bv %v for project %v, version %v at %v",
				buildvariant.Name, projectRef.String(), version.Id, activateAt)
		}

		version.BuildIds = append(version.BuildIds, buildId)
		version.BuildVariants = append(version.BuildVariants, BuildStatus{
			BuildVariant: buildvariant.Name,
			Activated:    false,
			ActivateAt:   activateAt,
			BuildId:      buildId,
		})
	}

	if err = version.Insert(); err != nil {
		for _, buildStatus := range version.BuildVariants {
			if buildErr := DeleteBuild(buildStatus.BuildId); buildErr != nil {
				mci.Logger.Errorf(slogger.ERROR, "Error deleting build %v: %v",
					buildStatus.BuildId, buildErr)
			}
		}
		return nil, err
	}
	return version, nil
}
