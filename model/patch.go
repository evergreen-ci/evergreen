package model

import (
	"10gen.com/mci"
	"10gen.com/mci/command"
	"10gen.com/mci/db"
	"10gen.com/mci/db/bsonutil"
	"10gen.com/mci/thirdparty"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"gopkg.in/yaml.v1"
	"io/ioutil"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	PatchCollection = "patches"
)

// Stores all details related to a patch request
type Patch struct {
	Id            bson.ObjectId `bson:"_id,omitempty"`
	Description   string        `bson:"desc"`
	Project       string        `bson:"branch"`
	Githash       string        `bson:"githash"`
	PatchNumber   int           `bson:"patch_number"`
	Author        string        `bson:"author"`
	Version       string        `bson:"version"`
	Status        string        `bson:"status"`
	CreateTime    time.Time     `bson:"create_time"`
	StartTime     time.Time     `bson:"start_time"`
	FinishTime    time.Time     `bson:"finish_time"`
	BuildVariants []string      `bson:"build_variants"`
	Tasks         []string      `bson:"tasks"`
	Patches       []ModulePatch `bson:"patches"`
	Activated     bool          `bson:"activated"`
}

// this stores request details for a patch
type ModulePatch struct {
	ModuleName string   `bson:"name"`
	Githash    string   `bson:"githash"`
	PatchSet   PatchSet `bson:"patch_set"`
}

// this stores information about the actual patch
type PatchSet struct {
	Patch   string               `bson:"patch"`
	Summary []thirdparty.Summary `bson:"summary"`
}

// A struct with which we process patch requests
type PatchAPIRequest struct {
	ProjectFileName string
	ModuleName      string
	Githash         string
	PatchContent    string
	BuildVariants   []string
}

type PatchMetadata struct {
	Githash       string
	Project       *Project
	Module        *Module
	BuildVariants []string
	Summaries     []thirdparty.Summary
}

var (
	// BSON fields for the patch struct
	PatchIdKey            = bsonutil.MustHaveTag(Patch{}, "Id")
	PatchDescriptionKey   = bsonutil.MustHaveTag(Patch{}, "Description")
	PatchProjectKey       = bsonutil.MustHaveTag(Patch{}, "Project")
	PatchGithashKey       = bsonutil.MustHaveTag(Patch{}, "Githash")
	PatchAuthorKey        = bsonutil.MustHaveTag(Patch{}, "Author")
	PatchNumberKey        = bsonutil.MustHaveTag(Patch{}, "PatchNumber")
	PatchVersionKey       = bsonutil.MustHaveTag(Patch{}, "Version")
	PatchStatusKey        = bsonutil.MustHaveTag(Patch{}, "Status")
	PatchCreateTimeKey    = bsonutil.MustHaveTag(Patch{}, "CreateTime")
	PatchStartTimeKey     = bsonutil.MustHaveTag(Patch{}, "StartTime")
	PatchFinishTimeKey    = bsonutil.MustHaveTag(Patch{}, "FinishTime")
	PatchBuildVariantsKey = bsonutil.MustHaveTag(Patch{}, "BuildVariants")
	PatchTasksKey         = bsonutil.MustHaveTag(Patch{}, "Tasks")
	PatchPatchesKey       = bsonutil.MustHaveTag(Patch{}, "Patches")
	PatchActivatedKey     = bsonutil.MustHaveTag(Patch{}, "Activated")

	// BSON fields for the module patch struct
	ModulePatchNameKey    = bsonutil.MustHaveTag(ModulePatch{}, "ModuleName")
	ModulePatchGithashKey = bsonutil.MustHaveTag(ModulePatch{}, "Githash")
	ModulePatchSetKey     = bsonutil.MustHaveTag(ModulePatch{}, "PatchSet")

	// BSON fields for the patch set struct
	PatchSetPatchKey   = bsonutil.MustHaveTag(PatchSet{}, "Patch")
	PatchSetSummaryKey = bsonutil.MustHaveTag(PatchSet{}, "Summary")

	// BSON fields for the git patch summary struct
	GitSummaryNameKey      = bsonutil.MustHaveTag(thirdparty.Summary{}, "Name")
	GitSummaryAdditionsKey = bsonutil.MustHaveTag(thirdparty.Summary{}, "Additions")
	GitSummaryDeletionsKey = bsonutil.MustHaveTag(thirdparty.Summary{}, "Deletions")
)

/******************************************************
Find
******************************************************/
func FindOnePatch(query interface{}, projection interface{}) (*Patch, error) {
	patch := &Patch{}
	err := db.FindOne(
		PatchCollection,
		query,
		projection,
		db.NoSort,
		patch,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return patch, err
}

func FindAllPatches(query interface{}, projection interface{}, sort []string,
	skip int, limit int) ([]Patch, error) {
	patches := []Patch{}
	err := db.FindAll(
		PatchCollection,
		query,
		projection,
		sort,
		skip,
		limit,
		&patches,
	)
	return patches, err
}

func FindAllPatchesByProject(project string, sort []string, skip int,
	limit int) ([]Patch, error) {
	return FindAllPatches(
		bson.M{
			PatchProjectKey: project,
		},
		db.NoProjection,
		sort,
		skip,
		limit,
	)
}

func FindPatchesByUser(user string, sort []string, skip int,
	limit int) ([]Patch, error) {
	return FindAllPatches(
		bson.M{
			PatchAuthorKey: user,
		},
		db.NoProjection,
		sort,
		skip,
		limit,
	)
}

func FindPatchByUserProjectGitspec(user string, project string,
	githash string) (*Patch, error) {
	return FindOnePatch(
		bson.M{
			PatchAuthorKey:  user,
			PatchProjectKey: project,
			PatchGithashKey: githash,
		},
		db.NoProjection,
	)
}

func FindExistingPatch(id string) (*Patch, error) {
	if bson.IsObjectIdHex(id) {
		return FindOnePatch(
			bson.M{
				PatchIdKey: bson.ObjectIdHex(id),
			},
			db.NoProjection,
		)
	}
	return nil, nil
}

func FindPatchByVersion(version string) (*Patch, error) {
	return FindOnePatch(
		bson.M{
			PatchVersionKey: version,
		},
		db.NoProjection,
	)
}

/******************************************************
Update
******************************************************/

func UpdateAllPatches(query interface{},
	update interface{}) (info *mgo.ChangeInfo, err error) {
	return db.UpdateAll(
		PatchCollection,
		query,
		update,
	)
}

func UpdateOnePatch(query interface{}, update interface{}) error {
	return db.Update(
		PatchCollection,
		query,
		update,
	)
}

func (patch *Patch) SetDescription(desc string) error {
	patch.Description = desc
	return UpdateOnePatch(
		bson.M{
			PatchIdKey: patch.Id,
		},
		bson.M{
			"$set": bson.M{
				PatchDescriptionKey: desc,
			},
		},
	)
}

func (self *Patch) SetVariantsAndTasks(variants []string,
	tasks []string) error {
	self.Tasks = tasks
	self.BuildVariants = variants
	return UpdateOnePatch(
		bson.M{
			PatchIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				PatchBuildVariantsKey: variants,
				PatchTasksKey:         tasks,
			},
		},
	)
}

// TryMarkPatchStarted attempts to mark a patch as started if it
// isn't already marked as such
func TryMarkPatchStarted(versionId string, startTime time.Time) error {
	filter := bson.M{
		PatchVersionKey: versionId,
		PatchStatusKey:  mci.PatchCreated,
	}
	update := bson.M{
		"$set": bson.M{
			PatchStartTimeKey: startTime,
			PatchStatusKey:    mci.PatchStarted,
		},
	}
	err := UpdateOnePatch(filter, update)
	if err == mgo.ErrNotFound {
		return nil
	}
	return err
}

/******************************************************
Remove
******************************************************/
func RemovePatches(query interface{}) error {
	return db.Remove(
		PatchCollection,
		query,
	)
}

/******************************************************
Create
******************************************************/

func (self *Patch) Insert() error {
	return db.Insert(PatchCollection, self)
}

// ComputePatchNumber counts all patches in the db before the current patch's
// creation time and returns that count + 1
func (self *Patch) ComputePatchNumber() (int, error) {
	count, err := db.Count(
		PatchCollection,
		bson.M{
			PatchCreateTimeKey: bson.M{"$lt": self.CreateTime},
			PatchAuthorKey:     self.Author,
		},
	)
	return count + 1, err
}

// Given a patch version and a list of task names, creates a new task with
// the given name for each variant, if applicable.
func (self *Patch) AddNewTasks(mciSettings *mci.MCISettings, patchVersion *Version,
	taskNames []string) error {
	project, err := FindProject("", self.Project, mciSettings.ConfigDir)
	if err != nil {
		return err
	}

	session, db, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error establishing database connection: %v", err)
		return err
	}
	defer session.Close()

	change := mgo.Change{
		Update: bson.M{
			"$addToSet": bson.M{
				"tasks": bson.M{"$each": taskNames},
			},
		},
		ReturnNew: false,
	}

	_, err = db.C(PatchCollection).
		Find(bson.M{"_id": self.Id}).
		Apply(change, self)

	if err != nil {
		return err
	}

	var newTasks []string
	for _, taskName := range taskNames {
		if !util.SliceContains(self.Tasks, taskName) {
			newTasks = append(newTasks, taskName)
		}
	}

	if len(newTasks) > 0 {
		self.Tasks = append(self.Tasks, newTasks...)

		builds, err := FindAllBuilds(
			bson.M{
				BuildIdKey: bson.M{
					"$in": patchVersion.BuildIds,
				},
			},
			bson.M{},
			[]string{},
			0,
			0,
		)

		if err != nil {
			return err
		}

		for _, build := range builds {
			_, err = AddTasksToBuild(&build, project, patchVersion, newTasks, mciSettings)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Given the patch version and a list of build variants, creates new builds
// with the patch's tasks.
func (self *Patch) AddNewBuilds(mciSettings *mci.MCISettings,
	patchVersion *Version, buildVariants []string) (*Version, error) {
	project, err := FindProject("", self.Project, mciSettings.ConfigDir)
	if err != nil {
		return nil, err
	}

	session, db, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error establishing database connection: %v", err)
		return nil, err
	}
	defer session.Close()

	change := mgo.Change{
		Update: bson.M{
			"$addToSet": bson.M{
				"build_variants": bson.M{"$each": buildVariants},
			},
		},
		ReturnNew: false,
	}

	_, err = db.C(PatchCollection).
		Find(bson.M{"_id": self.Id}).
		Apply(change, self)

	if err != nil {
		return nil, err
	}

	var newVariants []string
	for _, variant := range buildVariants {
		if !util.SliceContains(self.BuildVariants, variant) {
			newVariants = append(newVariants, variant)
		}
	}

	self.BuildVariants = append(self.BuildVariants, newVariants...)

	newBuildIds := make([]string, 0)
	newBuildStatuses := make([]BuildStatus, 0)
	for _, buildVariant := range newVariants {
		mci.Logger.Logf(slogger.INFO,
			"Creating build for version %v, buildVariant %v, activated = %v",
			patchVersion.Id, buildVariant, self.Activated)
		buildId, err := CreateBuildFromVersion(project, patchVersion, buildVariant, self.Activated, self.Tasks, mciSettings)
		if err != nil {
			return nil, err
		}
		newBuildIds = append(newBuildIds, buildId)

		newBuildStatuses = append(newBuildStatuses,
			BuildStatus{
				BuildVariant: buildVariant,
				BuildId:      buildId,
				Activated:    self.Activated,
			},
		)
		patchVersion.BuildIds = append(patchVersion.BuildIds, buildId)
	}

	err = UpdateOneVersion(
		bson.M{
			VersionIdKey: patchVersion.Id,
		},
		bson.M{
			"$push": bson.M{
				VersionBuildIdsKey:      bson.M{"$each": newBuildIds},
				VersionBuildVariantsKey: bson.M{"$each": newBuildStatuses},
			},
		},
	)

	if err != nil {
		return nil, err
	}

	return patchVersion, nil
}

// ConfigChanged looks through the parts of the patch and returns true if the
// passed in remotePath is in the the name of the changed files that are part
// of the patch
func (patch *Patch) ConfigChanged(remotePath string) bool {
	for _, patchPart := range patch.Patches {
		if patchPart.ModuleName == "" {
			for _, summary := range patchPart.PatchSet.Summary {
				if summary.Name == remotePath {
					return true
				}
			}
			return false
		}
	}
	return false
}

// PatchConfig takes in the path to a remote configuration a strigified version
// of the current project and returns an unmarshalled version of the project
// with the patch applied
func (patch *Patch) PatchConfig(remoteConfigPath, projectConfig string) (
	*Project, error) {
	for _, patchPart := range patch.Patches {
		// we only need to patch the main project and not any other modules
		if patchPart.ModuleName != "" {
			continue
		}
		// write patch file
		patchFilePath, err := util.WriteToTempFile(patchPart.PatchSet.Patch)
		if err != nil {
			return nil, fmt.Errorf("could not write patch file: %v", err)
		}
		defer os.Remove(patchFilePath)
		// write project configuration
		configFilePath, err := util.WriteToTempFile(projectConfig)
		if err != nil {
			return nil, fmt.Errorf("could not write config file: %v", err)
		}
		defer os.Remove(configFilePath)

		// clean the working directory
		workingDirectory := filepath.Dir(patchFilePath)
		localConfigPath := filepath.Join(
			workingDirectory,
			remoteConfigPath,
		)
		parentDir := strings.Split(
			remoteConfigPath,
			string(os.PathSeparator),
		)[0]
		err = os.RemoveAll(filepath.Join(workingDirectory, parentDir))
		if err != nil {
			return nil, err
		}
		if err = os.MkdirAll(filepath.Dir(localConfigPath), 0755); err != nil {
			return nil, err
		}
		// rename the temporary config file name to the remote config
		// file path
		if err = os.Rename(configFilePath, localConfigPath); err != nil {
			return nil, fmt.Errorf("could not rename file '%v' to '%v': %v",
				configFilePath, localConfigPath, err)
		}
		defer os.Remove(localConfigPath)

		// selectively apply the patch to the config file
		patchCommandStrings := []string{
			fmt.Sprintf("set -o verbose"),
			fmt.Sprintf("set -o errexit"),
			fmt.Sprintf("git apply --whitespace=fix --include=%v < '%v'",
				remoteConfigPath, patchFilePath),
		}

		patchCmd := &command.LocalCommand{
			CmdString:        strings.Join(patchCommandStrings, "\n"),
			WorkingDirectory: workingDirectory,
			Stdout:           mci.NewInfoLoggingWriter(&mci.Logger),
			Stderr:           mci.NewErrorLoggingWriter(&mci.Logger),
			ScriptMode:       true,
		}
		if err = patchCmd.Run(); err != nil {
			return nil, fmt.Errorf("could not run patch command: %v", err)
		}
		// read in the patched config file
		data, err := ioutil.ReadFile(localConfigPath)
		if err != nil {
			return nil, fmt.Errorf("could not read patched config file: %v",
				err)
		}
		project := &Project{}
		if err = LoadProjectInto(data, project); err != nil {
			return nil, err
		}
		return project, nil
	}
	return nil, fmt.Errorf("no patch on project")
}

// Finalizes a patch:
// Patches a remote project's configuration file if needed.
// Creates a version for this patch and links it.
// Creates builds based on the version.
func (patch *Patch) Finalize(gitCommit *thirdparty.CommitEvent,
	mciSettings *mci.MCISettings, project *Project) (
	patchVersion *Version, err error) {
	// marshall the project YAML for storage
	projectYamlBytes, err := yaml.Marshal(project)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling patched project config from "+
			"repository revision “%v”: %v", patch.Githash, err)
	}

	patchVersion = &Version{
		// TODO version IDs for versions created from patches?
		Id:            fmt.Sprintf("%v_%v", patch.Id.Hex(), 0),
		CreateTime:    time.Now(),
		Project:       patch.Project,
		Revision:      patch.Githash,
		Author:        gitCommit.Commit.Committer.Name,
		AuthorEmail:   gitCommit.Commit.Committer.Email,
		Message:       gitCommit.Commit.Message,
		BuildIds:      []string{},
		BuildVariants: []BuildStatus{},
		Config:        string(projectYamlBytes),
		Status:        mci.PatchCreated,
		Requester:     mci.PatchVersionRequester,
	}

	buildVariants := patch.BuildVariants
	if len(patch.BuildVariants) == 1 && patch.BuildVariants[0] == "all" {
		buildVariants = make([]string, 0)
		for _, buildVariant := range project.BuildVariants {
			if buildVariant.Disabled {
				continue
			}
			buildVariants = append(buildVariants, buildVariant.Name)
		}
	}

	for _, buildvariant := range buildVariants {
		//TODO get the correct commit data
		buildId, err := CreateBuildFromVersion(project, patchVersion,
			buildvariant, true, patch.Tasks, mciSettings)
		if err != nil {
			return nil, err
		}
		patchVersion.BuildIds = append(patchVersion.BuildIds, buildId)
		patchVersion.BuildVariants = append(patchVersion.BuildVariants, BuildStatus{
			BuildVariant: buildvariant,
			Activated:    true,
			BuildId:      buildId,
		})
	}

	if err = patchVersion.Insert(); err != nil {
		return nil, err
	}

	patch.Version = patchVersion.Id
	patch.Activated = true
	err = UpdateOnePatch(
		bson.M{
			PatchIdKey: patch.Id,
		},
		bson.M{
			"$set": bson.M{
				PatchActivatedKey: true,
				PatchVersionKey:   patchVersion.Id,
			},
		},
	)
	if err != nil {
		return nil, err
	}
	return patchVersion, nil
}

func (self *Patch) Cancel() error {
	if self.Version != "" {
		if err := SetPatchVersionActivated(self.Version, false); err != nil {
			return err
		}
		version, err := FindVersion(self.Version)
		if err != nil {
			return err
		}
		return version.SetAllTasksAborted(true)
	} else {
		return RemovePatches(
			bson.M{
				PatchIdKey: self.Id,
			},
		)
	}
}

func (self *PatchAPIRequest) Validate(configName string,
	oauthToken string) (*PatchMetadata, string, error) {
	var err error
	var repoOwner, repo string
	var module *Module

	// validate the project file
	project, err := FindProject("", self.ProjectFileName, configName)
	if err != nil {
		return nil, "", fmt.Errorf("Could not find project file %v: %v",
			self.ProjectFileName, err)
	}
	if project == nil {
		return nil, "No such project file named:" + self.ProjectFileName, nil
	}
	repoOwner = project.Owner
	repo = project.Repo

	if self.ModuleName != "" {
		// is there a module? validate it.
		module, err = project.GetModuleByName(self.ModuleName)
		if err != nil {
			return nil, "", fmt.Errorf("Could not find module %v: %v",
				self.ModuleName, err)
		}
		if module == nil {
			return nil, "no module named: " + self.ModuleName, nil
		}
		repoOwner, repo = module.GetRepoOwnerAndName()
	}

	if len(self.Githash) != 40 {
		return nil, "invalid githash", nil
	}
	gitCommit, err := thirdparty.GetCommitEvent(oauthToken, repoOwner, repo,
		self.Githash)
	if err != nil {
		return nil, "", fmt.Errorf("Could not find base revision %v for "+
			"project %v: %v",
			self.Githash, project.Identifier, err)
	}
	if gitCommit == nil {
		return nil, fmt.Sprintf("commit hash %v doesn't seem to exist",
			self.Githash), nil
	}

	gitOutput, err := thirdparty.GitApplyNumstat(self.PatchContent)
	if err != nil {
		return nil, fmt.Sprintf("couldn't validate patch: %v", err.Error()),
			nil
	}
	if gitOutput == nil {
		return nil, "couldn't validate patch: git apply --numstat returned" +
			" empty.", nil
	}

	summaries, err := thirdparty.ParseGitSummary(gitOutput)
	if err != nil {
		return nil, fmt.Sprintf("couldn't validate patch: %v", err.Error()), nil
	}

	if len(self.BuildVariants) == 0 || self.BuildVariants[0] == "" {
		return nil, "No buildvariants specified", nil
	}

	// verify that this build variant exists
	for _, buildVariant := range self.BuildVariants {
		if buildVariant == "all" {
			continue
		}
		bv := project.FindBuildVariant(buildVariant)
		if bv == nil {
			return nil, fmt.Sprintf("No such buildvariant: %v", buildVariant),
				nil
		}
	}
	return &PatchMetadata{self.Githash, project, module, self.BuildVariants,
		summaries}, "", nil
}

// Add or update a module within a patch.
func (self *Patch) UpdateModulePatch(modulePatch ModulePatch) error {
	// check that a patch for this module exists
	query := bson.M{
		PatchIdKey: self.Id,
		PatchPatchesKey + "." + ModulePatchNameKey: modulePatch.ModuleName,
	}
	update := bson.M{
		"$set": bson.M{
			PatchPatchesKey + ".$": modulePatch,
		},
	}
	result, err := UpdateAllPatches(query, update)
	if err != nil {
		return err
	}

	// The patch already existed in the array, and it's been updated.
	if result.Updated > 0 {
		return nil
	}

	//it wasn't in the array, we need to add it.
	query = bson.M{
		PatchIdKey: self.Id,
	}
	update = bson.M{
		"$push": bson.M{
			PatchPatchesKey: modulePatch,
		},
	}
	return UpdateOnePatch(query, update)
}

// removes a module that's part of a patch request
func (self *Patch) RemoveModulePatch(moduleName string) error {
	// check that a patch for this module exists
	query := bson.M{
		PatchIdKey: self.Id,
	}
	update := bson.M{
		"$pull": bson.M{
			PatchPatchesKey: bson.M{
				ModulePatchNameKey: moduleName,
			},
		},
	}

	return UpdateOnePatch(query, update)
}
