package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/db/bsonutil"
	"10gen.com/mci/model/patch"
	"10gen.com/mci/model/version"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/shelman/angier"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"strconv"
	"strings"
	"time"
)

const (
	BuildsCollection = "builds"
	AllDependencies  = "*"
)

// some duped information about tasks, mainly for ui purposes
type TaskCache struct {
	Id          string        `bson:"id" json:"id"`
	DisplayName string        `bson:"d" json:"display_name"`
	Status      string        `bson:"s" json:"status"`
	StartTime   time.Time     `bson:"st" json:"start_time"`
	TimeTaken   time.Duration `bson:"tt" json:"time_taken"`
	Activated   bool          `bson:"a" json:"activated"`
}

type Build struct {
	Id                  string        `bson:"_id" json:"_id"`
	CreateTime          time.Time     `bson:"create_time" json:"create_time,omitempty"`
	StartTime           time.Time     `bson:"start_time" json:"start_time,omitempty"`
	FinishTime          time.Time     `bson:"finish_time" json:"finish_time,omitempty"`
	PushTime            time.Time     `bson:"push_time" json:"push_time,omitempty"`
	Version             string        `bson:"version" json:"version,omitempty"`
	Project             string        `bson:"branch" json:"branch,omitempty"`
	Revision            string        `bson:"gitspec" json:"gitspec,omitempty"`
	BuildVariant        string        `bson:"build_variant" json:"build_variant,omitempty"`
	BuildNumber         string        `bson:"build_number" json:"build_number,omitempty"`
	Status              string        `bson:"status" json:"status,omitempty"`
	Activated           bool          `bson:"activated" json:"activated,omitempty"`
	ActivatedTime       time.Time     `bson:"activated_time" json:"activated_time,omitempty"`
	RevisionOrderNumber int           `bson:"order,omitempty" json:"order,omitempty"`
	Tasks               []TaskCache   `bson:"tasks" json:"tasks,omitempty"`
	TimeTaken           time.Duration `bson:"time_taken" json:"time_taken,omitempty"`
	DisplayName         string        `bson:"display_name" json:"display_name,omitempty"`

	// build requester - this is used to help tell the
	// reason this build was created. e.g. it could be
	// because the repotracker requested it (via tracking the
	// repository) or it was triggered by a developer
	// patch request
	Requester string `bson:"r" json:"r,omitempty"`
}

var (
	// bson fields for the build struct
	BuildIdKey                  = bsonutil.MustHaveTag(Build{}, "Id")
	BuildCreateTimeKey          = bsonutil.MustHaveTag(Build{}, "CreateTime")
	BuildStartTimeKey           = bsonutil.MustHaveTag(Build{}, "StartTime")
	BuildFinishTimeKey          = bsonutil.MustHaveTag(Build{}, "FinishTime")
	BuildPushTimeKey            = bsonutil.MustHaveTag(Build{}, "PushTime")
	BuildVersionKey             = bsonutil.MustHaveTag(Build{}, "Version")
	BuildProjectKey             = bsonutil.MustHaveTag(Build{}, "Project")
	BuildRevisionKey            = bsonutil.MustHaveTag(Build{}, "Revision")
	BuildBuildVariantKey        = bsonutil.MustHaveTag(Build{}, "BuildVariant")
	BuildBuildNumberKey         = bsonutil.MustHaveTag(Build{}, "BuildNumber")
	BuildStatusKey              = bsonutil.MustHaveTag(Build{}, "Status")
	BuildActivatedKey           = bsonutil.MustHaveTag(Build{}, "Activated")
	BuildActivatedTimeKey       = bsonutil.MustHaveTag(Build{}, "ActivatedTime")
	BuildRevisionOrderNumberKey = bsonutil.MustHaveTag(Build{}, "RevisionOrderNumber")
	BuildTasksKey               = bsonutil.MustHaveTag(Build{}, "Tasks")
	BuildTimeTakenKey           = bsonutil.MustHaveTag(Build{}, "TimeTaken")
	BuildDisplayNameKey         = bsonutil.MustHaveTag(Build{}, "DisplayName")
	BuildRequesterKey           = bsonutil.MustHaveTag(Build{}, "Requester")

	// bson fields for the task caches
	TaskCacheIdKey          = bsonutil.MustHaveTag(TaskCache{}, "Id")
	TaskCacheDisplayNameKey = bsonutil.MustHaveTag(TaskCache{}, "DisplayName")
	TaskCacheStatusKey      = bsonutil.MustHaveTag(TaskCache{}, "Status")
	TaskCacheStartTimeKey   = bsonutil.MustHaveTag(TaskCache{}, "StartTime")
	TaskCacheTimeTakenKey   = bsonutil.MustHaveTag(TaskCache{}, "TimeTaken")
	TaskCacheActivatedKey   = bsonutil.MustHaveTag(TaskCache{}, "Activated")
)

// Creates a new task cache with the specified id, display name, and value for
// activated.
func NewTaskCache(id string, displayName string, activated bool) TaskCache {
	return TaskCache{
		Id:          id,
		DisplayName: displayName,
		Status:      mci.TaskUndispatched,
		StartTime:   ZeroTime,
		TimeTaken:   time.Duration(0),
		Activated:   activated,
	}
}

// Returns whether or not the build has finished, based on its status.
func (build *Build) IsFinished() bool {
	return build.Status == mci.BuildFailed ||
		build.Status == mci.BuildCancelled ||
		build.Status == mci.BuildSucceeded
}

/*************************
Find
*************************/

func FindOneBuild(query interface{}, projection interface{},
	sort []string) (*Build, error) {
	build := &Build{}
	err := db.FindOne(
		BuildsCollection,
		query,
		projection,
		sort,
		build,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return build, err
}

func FindAllBuilds(query interface{}, projection interface{}, sort []string,
	skip int, limit int) ([]Build, error) {
	builds := []Build{}
	err := db.FindAll(
		BuildsCollection,
		query,
		projection,
		sort,
		skip,
		limit,
		&builds,
	)
	return builds, err
}

func FindBuild(id string) (*Build, error) {
	return FindOneBuild(
		bson.M{
			BuildIdKey: id,
		},
		db.NoProjection,
		db.NoSort,
	)
}

func FindDisplayName(buildVariant string) (*Build, error) {
	return FindOneBuild(
		bson.M{
			BuildBuildVariantKey: buildVariant,
		},
		db.NoProjection,
		db.NoSort,
	)
}

func (self *Build) FindBuildOnBaseCommit() (*Build, error) {
	return FindOneBuild(
		bson.M{
			BuildRevisionKey:     self.Revision,
			BuildRequesterKey:    mci.RepotrackerVersionRequester,
			BuildBuildVariantKey: self.BuildVariant,
		},
		db.NoProjection,
		db.NoSort,
	)
}

// Find all builds on the same project + build variant + requester between
// the current build and the specified previous build.
func (current *Build) FindIntermediateBuilds(previous *Build) ([]Build, error) {
	intermediateRevisions := bson.M{
		"$lt": current.RevisionOrderNumber,
		"$gt": previous.RevisionOrderNumber,
	}

	intermediateBuilds, err := FindAllBuilds(
		bson.M{
			BuildBuildVariantKey:        current.BuildVariant,
			BuildRequesterKey:           current.Requester,
			BuildRevisionOrderNumberKey: intermediateRevisions,
			BuildProjectKey:             current.Project,
		},
		db.NoProjection,
		[]string{BuildRevisionOrderNumberKey},
		db.NoSkip,
		db.NoLimit,
	)

	if err != nil {
		return nil, err
	}

	return intermediateBuilds, err
}

// Find the most recent activated build with the same build variant +
// requester + project as the current build.
func (current *Build) PreviousActivatedBuild(project string,
	requester string) (*Build, error) {

	return FindOneBuild(
		bson.M{
			BuildRevisionOrderNumberKey: bson.M{
				"$lt": current.RevisionOrderNumber,
			},
			BuildActivatedKey:    true,
			BuildBuildVariantKey: current.BuildVariant,
			BuildProjectKey:      project,
			BuildRequesterKey:    requester,
		},
		db.NoProjection,
		[]string{"-" + BuildRevisionOrderNumberKey},
	)

}

// Find the most recent build on with the same build variant + requester +
// project as the current build, with any of the specified statuses.
func (build *Build) GetPriorBuildWithStatuses(statuses []string) (*Build, error) {

	return FindOneBuild(
		bson.M{
			BuildRevisionOrderNumberKey: bson.M{
				"$lt": build.RevisionOrderNumber,
			},
			BuildBuildVariantKey: build.BuildVariant,
			BuildRequesterKey:    mci.RepotrackerVersionRequester,
			BuildStatusKey: bson.M{
				"$in": statuses,
			},
			BuildProjectKey: build.Project,
		},
		db.NoProjection,
		[]string{"-" + BuildRevisionOrderNumberKey},
	)

}

// Find all of the builds for the specified requester + project that finished
// after the specified time.
func RecentlyFinishedBuilds(finishTime time.Time, project string,
	requester string) ([]Build, error) {

	query := bson.M{}
	andClause := []bson.M{}

	// filter by finished builds
	finishedOpts := bson.M{
		BuildTimeTakenKey: bson.M{
			"$ne": time.Duration(0),
		},
	}

	// filter by finish_time
	timeOpt := bson.M{
		BuildFinishTimeKey: bson.M{
			"$gt": finishTime,
		},
	}

	// filter by requester
	requesterOpt := bson.M{
		BuildRequesterKey: requester,
	}

	// build query
	andClause = append(andClause, finishedOpts)
	andClause = append(andClause, timeOpt)
	andClause = append(andClause, requesterOpt)

	// filter by project
	if project != "" {
		projectOpt := bson.M{
			BuildProjectKey: project,
		}
		andClause = append(andClause, projectOpt)
	}

	query["$and"] = andClause

	return FindAllBuilds(
		query,
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

/***********************
Update
***********************/

func UpdateOneBuild(query interface{}, update interface{}) error {
	return db.Update(
		BuildsCollection,
		query,
		update,
	)
}

func (self *Build) UpdateStatus(status string) error {
	self.Status = status
	return UpdateOneBuild(
		bson.M{
			BuildIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				BuildStatusKey: status,
			},
		},
	)
}

func (self *Build) SetPriority(priority int) error {
	modifier := bson.M{TaskPriorityKey: priority}

	//blacklisted - this build should never run, so unschedule it now
	if priority < 0 {
		modifier[TaskActivatedKey] = false
	}

	_, err := UpdateAllTasks(
		bson.M{
			TaskBuildIdKey: self.Id,
		},
		bson.M{"$set": modifier},
	)
	return err
}

// TryMarkPatchFinished attempts to mark a patch as finished if all
// the builds for the patch are finished as well
func (build *Build) TryMarkPatchFinished(finishTime time.Time) error {
	patchCompleted := true
	status := mci.PatchSucceeded

	v, err := version.FindOne(version.ById(build.Version))
	if err != nil {
		return err
	}
	if v == nil {
		return fmt.Errorf("Can not find version for build %v with version %v", build.Id, build.Version)
	}

	// ensure all builds for this patch are finished as well
	builds, err := FindAllBuilds(
		bson.M{
			BuildIdKey: bson.M{"$in": v.BuildIds},
		},
		bson.M{
			BuildStatusKey: 1,
		},
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
	if err != nil {
		return err
	}

	for _, build := range builds {
		if !build.IsFinished() {
			patchCompleted = false
		}
		if build.Status != mci.BuildSucceeded {
			status = mci.PatchFailed
		}
	}

	// nothing to do if the patch isn't completed
	if !patchCompleted {
		return nil
	}

	return patch.TryMarkFinished(v.Id, finishTime, status)
}

// TryMarkBuildStarted attempts to mark a build as started if it
// isn't already marked as such
func TryMarkBuildStarted(buildId string, startTime time.Time) error {
	selector := bson.M{
		BuildIdKey:     buildId,
		BuildStatusKey: mci.BuildCreated,
	}

	update := bson.M{
		"$set": bson.M{
			BuildStatusKey:    mci.BuildStarted,
			BuildStartTimeKey: startTime,
		},
	}

	err := UpdateOneBuild(selector, update)
	if err == mgo.ErrNotFound {
		return nil
	}
	return err
}

func (self *Build) MarkFinished(status string, finishTime time.Time) error {
	self.Status = status
	self.FinishTime = finishTime
	self.TimeTaken = finishTime.Sub(self.StartTime)
	return UpdateOneBuild(
		bson.M{
			BuildIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				BuildStatusKey:     status,
				BuildFinishTimeKey: finishTime,
				BuildTimeTakenKey:  self.TimeTaken,
			},
		},
	)
}

/***********************
Remove
***********************/

func RemoveBuild(id string) error {
	return db.Remove(
		BuildsCollection,
		bson.M{
			BuildIdKey: id,
		},
	)
}

// Delete any record of the build, by removing it and all of the tasks that
// are a part of it from the database.
func DeleteBuild(id string) error {
	err := RemoveAllTasks(
		bson.M{
			TaskBuildIdKey: id,
		},
	)

	if err != nil && err != mgo.ErrNotFound {
		return err
	}

	return RemoveBuild(id)
}

/***********************
Create
***********************/

func (self *Build) Insert() error {
	return db.Insert(BuildsCollection, self)
}

func AddTasksToBuild(build *Build, project *Project, v *version.Version, taskNames []string) (*Build, error) {

	// find the build variant for this project/build
	buildVariant := project.FindBuildVariant(build.BuildVariant)
	if buildVariant == nil {
		return nil, fmt.Errorf("Could not find build %v in %v:%v project file",
			build.BuildVariant, project.RepoKind, project.Identifier)
	}

	// create the new tasks for the build
	tasks, err := createTasksForBuild(project, buildVariant, build, v, taskNames)
	if err != nil {
		return nil, fmt.Errorf("error creating tasks for build %v: %v",
			build.Id, err)
	}

	// insert the tasks into the db
	for _, task := range tasks {
		mci.Logger.Logf(slogger.INFO, "Creating task “%v”", task.DisplayName)
		if err := task.Insert(); err != nil {
			return nil, fmt.Errorf("error inserting task %v: %v", task.Id, err)
		}
	}

	// create task caches for all of the tasks, and add them into the build
	for _, task := range tasks {
		cache := &TaskCache{}
		if err = angier.TransferByFieldNames(task, cache); err != nil {
			return nil, fmt.Errorf("Could not merge task into cache: %v", err)
		}
		build.Tasks = append(build.Tasks, *cache)
	}

	// TODO: re-sort the task caches?

	// update the build to hold the new tasks
	err = UpdateOneBuild(
		bson.M{"_id": build.Id},
		bson.M{
			"$set": bson.M{
				"tasks": build.Tasks,
			},
		})

	if err != nil {
		return nil, err
	}

	return build, nil
}

// Create a build, given all of the necessary information from the corresponding
// version and project.
func CreateBuildFromVersion(project *Project, v *version.Version, buildName string,
	activated bool, taskNames []string) (string, error) {

	mci.Logger.Logf(slogger.DEBUG, "Creating %v %v build, activated: %v", v.Requester, buildName, activated)

	// find the build variant for this project/build
	buildVariant := project.FindBuildVariant(buildName)
	if buildVariant == nil {
		return "", fmt.Errorf("Could not find build %v in %v:%v project file",
			buildName, project.RepoKind, project.Identifier)
	}

	// get a new build id
	buildId := CleanName(fmt.Sprintf("%v_%v_%v_%v", project.Identifier, buildName,
		v.Revision, v.CreateTime.Format("06_01_02_15_04_05")))

	// create the build itself
	build := &Build{
		Id:                  buildId,
		CreateTime:          v.CreateTime,
		PushTime:            v.CreateTime,
		Activated:           activated,
		Project:             project.Identifier,
		Revision:            v.Revision,
		Status:              mci.BuildCreated,
		BuildVariant:        buildName,
		Version:             v.Id,
		DisplayName:         buildVariant.DisplayName,
		RevisionOrderNumber: v.RevisionOrderNumber,
		Requester:           v.Requester,
	}

	// get a new build number for the build
	buildNumber, err := db.GetNewBuildVariantBuildNumber(buildName)
	if err != nil {
		return "", fmt.Errorf("Could not get build number for build variant"+
			" %v in %v:%v project file: %v", buildName, project.RepoKind,
			project.Identifier)
	}
	build.BuildNumber = strconv.FormatUint(buildNumber, 10)

	// create all of the necessary tasks for the build
	tasksForBuild, err := createTasksForBuild(project, buildVariant, build, v, taskNames)
	if err != nil {
		return "", fmt.Errorf("error creating tasks for build %v: %v", build.Id,
			err)
	}

	// insert all of the build's tasks into the db
	for _, task := range tasksForBuild {
		if err := task.Insert(); err != nil {
			return "", fmt.Errorf("error inserting task %v: %v", task.Id, err)
		}
	}

	// create task caches for all of the tasks, and place them into the build
	build.Tasks = make([]TaskCache, 0, len(tasksForBuild))
	for _, task := range tasksForBuild {
		cache := &TaskCache{}
		angier.TransferByFieldNames(task, cache)
		build.Tasks = append(build.Tasks, *cache)
	}

	// insert the build
	if err := build.Insert(); err != nil {
		return "", fmt.Errorf("error inserting build %v: %v", build.Id, err)
	}

	// success!
	return build.Id, nil
}

// create all of the necessary tasks for the build.  returns a
// slice of all of the tasks created, as well as an error if any occurs.
// the slice of tasks will be in the same order as the project's specified tasks
// appear in the specified build variant.
func createTasksForBuild(project *Project, buildVariant *BuildVariant,
	build *Build, v *version.Version, taskNames []string) ([]*Task, error) {

	// the list of tasks we should create.  if tasks are passed in, then
	// use those, else use the default set
	tasksToCreate := []BuildVariantTask{}
	createAll := len(taskNames) == 0
	for _, task := range buildVariant.Tasks {
		if task.Name == mci.PushStage &&
			build.Requester == mci.PatchVersionRequester {
			continue
		}
		if createAll || util.SliceContains(taskNames, task.Name) {
			tasksToCreate = append(tasksToCreate, task)
		}
	}

	// create a map of display name -> task id for all of the tasks we are
	// going to create.  we do this ahead of time so we can access it for the
	// dependency lists.
	taskIdsByDisplayName := map[string]string{}
	for _, task := range tasksToCreate {
		taskId := CleanName(fmt.Sprintf("%v_%v_%v", build.Id, task.Name,
			buildVariant.Name))
		taskIdsByDisplayName[task.Name] = taskId
	}

	// if any tasks already exist in the build, add them to the map
	// so they can be used as dependencies
	for _, task := range build.Tasks {
		taskIdsByDisplayName[task.DisplayName] = task.Id
	}

	// create and insert all of the actual tasks
	tasks := make([]*Task, 0, len(tasksToCreate))
	for _, task := range tasksToCreate {

		// get the task spec out of the project
		var taskSpec ProjectTask
		for _, projectTask := range project.Tasks {
			if projectTask.Name == task.Name {
				taskSpec = projectTask
				break
			}
		}

		// sanity check that the config isn't malformed
		if taskSpec.Name == "" {
			return nil, fmt.Errorf("config is malformed: variant '%v' runs "+
				"task called '%v' but no such task exists for repo %v for "+
				"version %v", buildVariant.Name, task.Name, project.Identifier,
				v.Id)
		}

		// create the task
		newTask, err := createOneTask(taskIdsByDisplayName[task.Name], task,
			project, buildVariant, build, v)
		if err != nil {
			return nil, fmt.Errorf("error creating task: %v", err)
		}

		// set the tasks dependencies
		if len(taskSpec.DependsOn) == 1 &&
			taskSpec.DependsOn[0].Name == AllDependencies {
			// the task depends on all of the other tasks in the build
			newTask.DependsOn = make([]string, 0, len(tasksToCreate)-1)
			for _, dep := range tasksToCreate {
				if dep.Name != newTask.DisplayName {
					newTask.DependsOn = append(newTask.DependsOn,
						taskIdsByDisplayName[dep.Name])
				}
			}

		} else {
			// the task has specific dependencies
			newTask.DependsOn = make([]string, 0, len(taskSpec.DependsOn))
			for _, dep := range taskSpec.DependsOn {
				// only add as a dependency if the dependency is being created
				if taskIdsByDisplayName[dep.Name] != "" {
					newTask.DependsOn = append(newTask.DependsOn,
						taskIdsByDisplayName[dep.Name])
				}
			}
		}

		// append the task to the list of the created tasks
		tasks = append(tasks, newTask)

	}

	// return all of the tasks created
	return tasks, nil

}

// helper to create a single task
func createOneTask(id string, buildVarTask BuildVariantTask, project *Project,
	buildVariant *BuildVariant, build *Build, v *version.Version) (*Task, error) {

	// set all of the basic fields
	task := &Task{
		Id:                  id,
		Secret:              util.RandomString(),
		DisplayName:         buildVarTask.Name,
		BuildId:             build.Id,
		DistroId:            "",
		BuildVariant:        buildVariant.Name,
		CreateTime:          build.CreateTime,
		PushTime:            build.PushTime,
		ScheduledTime:       ZeroTime,
		StartTime:           ZeroTime, // Certain time fields must be initialized
		FinishTime:          ZeroTime, // to our own ZeroTime value (which is
		DispatchTime:        ZeroTime, // Unix epoch 0, not Go's time.Time{})
		LastHeartbeat:       ZeroTime,
		Status:              mci.TaskUndispatched,
		Activated:           build.Activated,
		RevisionOrderNumber: v.RevisionOrderNumber,
		Requester:           v.Requester,
		Version:             v.Id,
		Revision:            v.Revision,
		Project:             project.Identifier,
	}

	return task, nil

}

/* --------------------- HELPER METHODS FOR CREATE BUILD --------------------- */

func TestTaskId(build *Build, test TestSuite, distroId string) string {
	return CleanName(fmt.Sprintf("%v_%v_%v", build.Id, test.Name, distroId))
}

func CleanName(name string) string {
	name = strings.Replace(name, "-", "_", -1)
	name = strings.Replace(name, " ", "_", -1)
	return name
}

/* Aggregation commands for generating reports on builds */

//GetBuildMaster fetches the most recent "depth" builds and
//returns the task info in their task caches, grouped by
//task display name and buildvariant.
func GetBuildmasterData(v version.Version, depth int) ([]bson.M, error) {
	session, db, err := db.GetGlobalSessionFactory().GetSession()
	defer session.Close()

	pipeline := db.C(BuildsCollection).Pipe(
		[]bson.M{
			bson.M{
				"$match": bson.M{
					BuildRequesterKey: mci.RepotrackerVersionRequester,
					BuildRevisionOrderNumberKey: bson.M{
						"$lt": v.RevisionOrderNumber,
					},
					BuildProjectKey: v.Project,
				},
			},
			bson.M{
				"$sort": bson.M{
					BuildRevisionOrderNumberKey: -1,
				},
			},
			bson.M{
				"$limit": depth,
			},
			bson.M{
				"$project": bson.M{
					BuildTasksKey: 1,
					"v":           "$build_variant",
					"s":           1,
					"n":           "$build_number",
				},
			},
			bson.M{
				"$unwind": "$tasks",
			},
			bson.M{
				"$project": bson.M{
					"_id": 0,
					"v":   1,
					"d":   "$tasks.d",
					"st":  "$tasks.s",
					"tt":  "$tasks.tt",
					"tk":  "$tasks.tk",
					"a":   "$tasks.a",
					"id":  "$tasks.id",
				},
			},
			bson.M{
				"$group": bson.M{
					"_id": bson.M{
						"v": "$v",
						"d": "$d",
					},
					"tests": bson.M{
						"$push": bson.M{
							"s":  "$st",
							"id": "$id",
							"tt": "$tt",
							"a":  "$a",
							"tk": "$tk",
						},
					},
				},
			},
		},
	)
	var output []bson.M
	err = pipeline.All(&output)
	if err != nil {
		return nil, err
	}
	return output, nil
}
