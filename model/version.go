package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

type Version struct {
	Id                  string               `bson:"_id" json:"id,omitempty"`
	CreateTime          time.Time            `bson:"create_time" json:"create_time,omitempty"`
	StartTime           time.Time            `bson:"start_time" json:"start_time,omitempty"`
	FinishTime          time.Time            `bson:"finish_time" json:"finish_time,omitempty"`
	Revision            string               `bson:"gitspec" json:"revision,omitempty"`
	Author              string               `bson:"author" json:"author,omitempty"`
	AuthorEmail         string               `bson:"author_email" json:"author_email,omitempty"`
	Message             string               `bson:"message" json:"message,omitempty"`
	Status              string               `bson:"status" json:"status,omitempty"`
	RevisionOrderNumber int                  `bson:"order,omitempty" json:"order,omitempty"`
	Config              string               `bson:"config" json:"config,omitempty"`
	ConfigUpdateNumber  int                  `bson:"config_number" json:"config_number,omitempty"`
	Ignored             bool                 `bson:"ignored" json:"ignored"`
	Owner               string               `bson:"owner_name" json:"owner_name,omitempty"`
	Repo                string               `bson:"repo_name" json:"repo_name,omitempty"`
	Branch              string               `bson:"branch_name" json:"branch_name,omitempty"`
	BuildVariants       []VersionBuildStatus `bson:"build_variants_status,omitempty" json:"build_variants_status,omitempty"`
	PeriodicBuildID     string               `bson:"periodic_build_id,omitempty" json:"periodic_build_id,omitempty"`

	// GitTags stores tags that were pushed to this version, while TriggeredByGitTag is for versions created by tags
	GitTags           []GitTag `bson:"git_tags,omitempty" json:"git_tags,omitempty"`
	TriggeredByGitTag GitTag   `bson:"triggered_by_git_tag,omitempty" json:"triggered_by_git_tag,omitempty"`

	// Parameters stores user-defined parameters
	Parameters []patch.Parameter `bson:"parameters,omitempty" json:"parameters,omitempty"`
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
	Errors   []string `bson:"errors,omitempty" json:"errors,omitempty"`
	Warnings []string `bson:"warnings,omitempty" json:"warnings,omitempty"`

	// AuthorID is an optional reference to the Evergreen user that authored
	// this comment, if they can be identified
	AuthorID string `bson:"author_id,omitempty" json:"author_id,omitempty"`

	SatisfiedTriggers []string `bson:"satisfied_triggers,omitempty" json:"satisfied_triggers,omitempty"`
	// Fields set if triggered by an upstream build
	TriggerID    string `bson:"trigger_id,omitempty" json:"trigger_id,omitempty"`
	TriggerType  string `bson:"trigger_type,omitempty" json:"trigger_type,omitempty"`
	TriggerEvent string `bson:"trigger_event,omitempty" json:"trigger_event,omitempty"`

	// this is only used for aggregations, and is not stored in the DB
	Builds []build.Build `bson:"build_variants,omitempty" json:"build_variants,omitempty"`
}

func (v *Version) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(v) }
func (v *Version) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, v) }

const defaultVersionLimit = 20

type GetVersionsOptions struct {
	StartAfter     int    `json:"start"`
	Requester      string `json:"requester"`
	Limit          int    `json:"limit"`
	Skip           int    `json:"skip"`
	IncludeBuilds  bool   `json:"include_builds"`
	IncludeTasks   bool   `json:"include_tasks"`
	ByBuildVariant string `json:"by_build_variant"`
}

func (v *Version) LastSuccessful() (*Version, error) {
	lastGreen, err := VersionFindOne(VersionBySuccessfulBeforeRevision(v.Identifier, v.RevisionOrderNumber).Sort(
		[]string{"-" + VersionRevisionOrderNumberKey}))
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving last successful version")
	}
	return lastGreen, nil
}

func (self *Version) UpdateBuildVariants() error {
	return VersionUpdateOne(
		bson.M{VersionIdKey: self.Id},
		bson.M{
			"$set": bson.M{
				VersionBuildVariantsKey: self.BuildVariants,
			},
		},
	)
}

func (self *Version) Insert() error {
	return db.Insert(VersionCollection, self)
}

func (v *Version) AddSatisfiedTrigger(definitionID string) error {
	if v.SatisfiedTriggers == nil {
		v.SatisfiedTriggers = []string{}
	}
	v.SatisfiedTriggers = append(v.SatisfiedTriggers, definitionID)
	return errors.Wrap(AddSatisfiedTrigger(v.Id, definitionID), "error adding satisfied trigger")
}

func (v *Version) UpdateStatus(newStatus string) error {
	if v == nil {
		return errors.New("version is nil")
	}
	if v.Status == newStatus {
		return nil
	}
	update := bson.M{
		"$set": bson.M{
			VersionStatusKey: newStatus,
		},
	}
	err := VersionUpdateOne(bson.M{VersionIdKey: v.Id}, update)
	if err != nil {
		return err
	}
	v.Status = newStatus
	event.LogVersionStateChangeEvent(v.Id, newStatus)
	return nil
}

// GetTimeSpent returns the total time_taken and makespan of a version for
// each task that has finished running
func (v *Version) GetTimeSpent() (time.Duration, time.Duration, error) {
	tasks, err := task.FindAllFirstExecution(task.ByVersion(v.Id).WithFields(task.TimeTakenKey, task.StartTimeKey, task.FinishTimeKey, task.DisplayOnlyKey, task.ExecutionKey))
	if err != nil {
		return 0, 0, errors.Wrapf(err, "can't get tasks for version '%s'", v.Id)
	}
	if tasks == nil {
		return 0, 0, errors.Errorf("no tasks found for version '%s'", v.Id)
	}

	timeTaken, makespan := task.GetTimeSpent(tasks)
	return timeTaken, makespan, nil
}

func GetVersionForCommitQueueItem(cq *commitqueue.CommitQueue, issue string) (*Version, error) {
	spot := cq.FindItem(issue)
	if spot == -1 {
		return nil, nil
	}

	return VersionFindOneId(cq.Queue[spot].Version)
}

// VersionBuildStatus stores metadata relating to each build
type VersionBuildStatus struct {
	BuildVariant     string                `bson:"build_variant" json:"id"`
	BuildId          string                `bson:"build_id,omitempty" json:"build_id,omitempty"`
	BatchTimeTasks   []BatchTimeTaskStatus `bson:"batchtime_tasks,omitempty" json:"batchtime_tasks,omitempty"`
	ActivationStatus `bson:",inline"`
}

type BatchTimeTaskStatus struct {
	TaskName         string `bson:"task_name" json:"task_name"`
	TaskId           string `bson:"task_id,omitempty" json:"task_id,omitempty"`
	ActivationStatus `bson:",inline"`
}

type ActivationStatus struct {
	Activated  bool      `bson:"activated" json:"activated"`
	ActivateAt time.Time `bson:"activate_at,omitempty" json:"activate_at,omitempty"`
}

func (s *ActivationStatus) ShouldActivate(now time.Time) bool {
	return !s.Activated && now.After(s.ActivateAt) && !s.ActivateAt.IsZero()
}

// VersionMetadata is used to pass information about upstream versions to downstream version creation
type VersionMetadata struct {
	Revision            Revision
	TriggerID           string
	TriggerType         string
	EventID             string
	TriggerDefinitionID string
	SourceVersion       *Version
	IsAdHoc             bool
	User                *user.DBUser
	Message             string
	Alias               string
	PeriodicBuildID     string
	RemotePath          string
	GitTag              GitTag
}

var (
	VersionBuildStatusVariantKey        = bsonutil.MustHaveTag(VersionBuildStatus{}, "BuildVariant")
	VersionBuildStatusActivatedKey      = bsonutil.MustHaveTag(VersionBuildStatus{}, "Activated")
	VersionBuildStatusActivateAtKey     = bsonutil.MustHaveTag(VersionBuildStatus{}, "ActivateAt")
	VersionBuildStatusBuildIdKey        = bsonutil.MustHaveTag(VersionBuildStatus{}, "BuildId")
	VersionBuildStatusBatchTimeTasksKey = bsonutil.MustHaveTag(VersionBuildStatus{}, "BatchTimeTasks")

	BatchTimeTaskStatusTaskNameKey  = bsonutil.MustHaveTag(BatchTimeTaskStatus{}, "TaskName")
	BatchTimeTaskStatusActivatedKey = bsonutil.MustHaveTag(BatchTimeTaskStatus{}, "Activated")
)

type DuplicateVersionsID struct {
	Hash      string `bson:"hash"`
	ProjectID string `bson:"project_id"`
}

type DuplicateVersions struct {
	ID       DuplicateVersionsID `bson:"_id"`
	Versions []Version           `bson:"versions"`
}

func VersionGetHistory(versionId string, N int) ([]Version, error) {
	v, err := VersionFindOne(VersionById(versionId))
	if err != nil {
		return nil, errors.WithStack(err)
	} else if v == nil {
		return nil, errors.Errorf("Version '%v' not found", versionId)
	}

	// Versions in the same push event, assuming that no two push events happen at the exact same time
	// Never want more than 2N+1 versions, so make sure we add a limit

	siblingVersions, err := VersionFind(db.Query(
		bson.M{
			VersionRevisionOrderNumberKey: v.RevisionOrderNumber,
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			VersionIdentifierKey: v.Identifier,
		}).WithoutFields(VersionConfigKey).Sort([]string{VersionRevisionOrderNumberKey}).Limit(2*N + 1))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	versionIndex := -1
	for i := 0; i < len(siblingVersions); i++ {
		if siblingVersions[i].Id == v.Id {
			versionIndex = i
		}
	}

	numSiblings := len(siblingVersions) - 1
	versions := siblingVersions

	if versionIndex < N {
		// There are less than N later versions from the same push event
		// N subsequent versions plus the specified one
		subsequentVersions, err := VersionFind(
			//TODO encapsulate this query in version pkg
			db.Query(bson.M{
				VersionRevisionOrderNumberKey: bson.M{"$gt": v.RevisionOrderNumber},
				VersionRequesterKey: bson.M{
					"$in": evergreen.SystemVersionRequesterTypes,
				},
				VersionIdentifierKey: v.Identifier,
			}).WithoutFields(VersionConfigKey).Sort([]string{VersionRevisionOrderNumberKey}).Limit(N - versionIndex))
		if err != nil {
			return nil, errors.WithStack(err)
		}

		// Reverse the second array so we have the versions ordered "newest one first"
		for i := 0; i < len(subsequentVersions)/2; i++ {
			subsequentVersions[i], subsequentVersions[len(subsequentVersions)-1-i] = subsequentVersions[len(subsequentVersions)-1-i], subsequentVersions[i]
		}

		versions = append(subsequentVersions, versions...)
	}

	if numSiblings-versionIndex < N {
		previousVersions, err := VersionFind(db.Query(bson.M{
			VersionRevisionOrderNumberKey: bson.M{"$lt": v.RevisionOrderNumber},
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			VersionIdentifierKey: v.Identifier,
		}).WithoutFields(VersionConfigKey).Sort([]string{fmt.Sprintf("-%v", VersionRevisionOrderNumberKey)}).Limit(N))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		versions = append(versions, previousVersions...)
	}

	return versions, nil
}

func GetVersionsWithOptions(projectName string, opts GetVersionsOptions) ([]Version, error) {
	projectId, err := FindIdForProject(projectName)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding id for project '%s'", projectName)
	}
	match := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey:  opts.Requester,
	}
	if opts.ByBuildVariant != "" {
		match[bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusVariantKey)] = opts.ByBuildVariant
	}

	if opts.StartAfter > 0 {
		match[VersionRevisionOrderNumberKey] = bson.M{"$lt": opts.StartAfter}
	}

	pipeline := []bson.M{bson.M{"$match": match}}
	pipeline = append(pipeline, bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}})
	project := bson.M{
		VersionCreateTimeKey:          1,
		VersionStartTimeKey:           1,
		VersionFinishTimeKey:          1,
		VersionRevisionKey:            1,
		VersionAuthorKey:              1,
		VersionAuthorEmailKey:         1,
		VersionMessageKey:             1,
		VersionStatusKey:              1,
		VersionBuildVariantsKey:       1,
		VersionErrorsKey:              1,
		VersionRevisionOrderNumberKey: 1,
		VersionRequesterKey:           1,
	}

	if !opts.IncludeBuilds { // add project to the pipeline as is
		pipeline = append(pipeline, bson.M{"$project": project})
	} else {
		if opts.ByBuildVariant != "" {
			// add a filter so we only have the build variant we want (we already know this exists from the match stage)
			project[VersionBuildVariantsKey] = bson.M{
				"$filter": bson.M{
					"input": "$" + VersionBuildVariantsKey,
					"as":    "item",
					"cond": bson.M{
						"$eq": []string{"$$item.build_variant", opts.ByBuildVariant},
					},
				},
			}
		}

		pipeline = append(pipeline, bson.M{"$project": project})
		lookupBuilds := bson.M{
			"from":         build.Collection,
			"localField":   bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusBuildIdKey),
			"foreignField": build.IdKey,
			"as":           "build_variants",
		}
		pipeline = append(pipeline, bson.M{"$lookup": lookupBuilds})

		if opts.ByBuildVariant != "" {
			// filter out versions that don't have this variant activated
			matchBuildVariantActivated := bson.M{
				bsonutil.GetDottedKeyName("build_variants", build.ActivatedKey): true,
			}
			pipeline = append(pipeline, bson.M{"$match": matchBuildVariantActivated})
		}

		// projectWithBuilds is initially the same as the first project but with build_variants instead of build_variants_status.
		projectWithBuilds := bson.M{
			VersionRevisionKey:            1,
			VersionErrorsKey:              1,
			VersionMessageKey:             1,
			VersionAuthorKey:              1,
			VersionRevisionOrderNumberKey: 1,
			VersionCreateTimeKey:          1,
			VersionStartTimeKey:           1,
			VersionFinishTimeKey:          1,
			VersionStatusKey:              1,
			"build_variants":              1,
		}
		projectWithoutTaskCache := bson.M{
			"build_variants.tasks": 0,
		}
		pipeline = append(pipeline, bson.M{"$project": projectWithBuilds})
		pipeline = append(pipeline, bson.M{"$project": projectWithoutTaskCache})
	}
	if opts.Skip != 0 {
		pipeline = append(pipeline, bson.M{"$skip": opts.Skip})
	}
	if opts.Limit == 0 {
		opts.Limit = defaultVersionLimit
	}
	pipeline = append(pipeline, bson.M{"$limit": opts.Limit})

	res := []Version{}

	if err = db.Aggregate(VersionCollection, pipeline, &res); err != nil {
		return nil, errors.Wrapf(err, "error aggregating versions and builds")
	}

	// TODO: need to either iterate the query or use a nested lookup in order to use ByTask
	if opts.IncludeTasks {
		buildIds := []string{}
		for _, v := range res {
			for _, b := range v.Builds {
				buildIds = append(buildIds, b.Id)
			}
		}
		tasks, err := task.Find(task.ByBuildIds(buildIds).WithFields(task.IdKey, task.DisplayNameKey, task.StatusKey, task.BuildIdKey))
		if err != nil {
			return nil, errors.Wrapf(err, "error finding tasks")
		}
		tasksByBuildId := map[string][]task.Task{}
		for i, t := range tasks {
			tasksByBuildId[t.BuildId] = append(tasksByBuildId[t.BuildId], tasks[i])
		}
		for i, v := range res {
			for j, b := range v.Builds {
				res[i].Builds[j].Tasks = CreateTasksCache(tasksByBuildId[b.Id])
			}
		}
	}

	return res, nil
}

type VersionsByCreateTime []Version

func (v VersionsByCreateTime) Len() int {
	return len(v)
}

func (v VersionsByCreateTime) Less(i, j int) bool {
	return v[i].CreateTime.Before(v[j].CreateTime)
}

func (v VersionsByCreateTime) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}
