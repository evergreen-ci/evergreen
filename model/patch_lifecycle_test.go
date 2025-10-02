package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
	patchTestConfig = testutil.TestConfig()
	remotePath      = "model/testdata/project.yml"
	patchedProject  = "mci-config"
	patchedRevision = "17746cb296670f53ee326deb83ac8cc4dffe55dd"
	patchFile       = "testdata/patch2.diff"
	patchOwner      = "evergreen-ci"
	patchRepo       = "evergreen"
	patchBranch     = "main"

	// newProjectPatchFile is a diff that adds a new project configuration file
	// located at newConfigFilePath.
	newProjectPatchFile = "testdata/project.diff"
	newConfigFilePath   = "model/testdata/project2.config"
)

func init() {
	current := testutil.GetDirectoryOfFile()
	patchFile = filepath.Join(current, patchFile)
	newProjectPatchFile = filepath.Join(current, newProjectPatchFile)
}

func clearAll(t *testing.T) {
	require.NoError(t, db.ClearCollections(manifest.Collection, ParserProjectCollection, ProjectRefCollection, patch.Collection, VersionCollection, build.Collection, task.Collection, distro.Collection))
}

// resetPatchSetup clears the ProjectRef, Patch, Version, Build, and Task Collections
// and creates a patch from the test path given. The patch is not inserted.
func resetPatchSetup(ctx context.Context, t *testing.T, testPath string) *patch.Patch {
	clearAll(t)
	projectRef := &ProjectRef{
		Id:         patchedProject,
		RemotePath: remotePath,
		Owner:      patchOwner,
		Repo:       patchRepo,
		Branch:     patchBranch,
	}
	// insert distros to be used
	distros := []distro.Distro{{Id: "d1"}, {Id: "d2"}}
	for _, d := range distros {
		err := d.Insert(ctx)
		require.NoError(t, err, "Couldn't insert test distro: %v", err)
	}

	err := projectRef.Insert(t.Context())
	require.NoError(t, err, "Couldn't insert test project ref: %v", err)

	baseVersion := &Version{
		Identifier: patchedProject,
		CreateTime: time.Now(),
		Revision:   patchedRevision,
		Requester:  evergreen.RepotrackerVersionRequester,
	}
	err = baseVersion.Insert(t.Context())
	require.NoError(t, err, "Couldn't insert test base version: %v", err)

	fileBytes, err := os.ReadFile(patchFile)
	require.NoError(t, err, "Couldn't read patch file: %v", err)

	// this patch adds a new task to the existing build
	configPatch := &patch.Patch{
		Id:            mgobson.NewObjectId(),
		Project:       patchedProject,
		Githash:       patchedRevision,
		Tasks:         []string{"taskTwo", "taskOne"},
		BuildVariants: []string{"linux-64-duroff"},
		Parameters: []patch.Parameter{
			{Key: "my_param", Value: "is_this"},
		},
		Patches: []patch.ModulePatch{
			{
				Githash: "revision",
				PatchSet: patch.PatchSet{
					Patch: fmt.Sprintf(string(fileBytes), testPath, testPath, testPath, testPath),
					Summary: []thirdparty.Summary{
						{Name: remotePath, Additions: 4, Deletions: 80},
						{Name: "random.txt", Additions: 6, Deletions: 0},
					},
				},
			},
		},
	}

	return configPatch
}

func resetProjectlessPatchSetup(ctx context.Context, t *testing.T) *patch.Patch {
	clearAll(t)
	projectRef := &ProjectRef{
		Id:         patchedProject,
		RemotePath: newConfigFilePath,
		Owner:      patchOwner,
		Repo:       patchRepo,
		Branch:     patchBranch,
	}
	// insert distros to be used
	distros := []distro.Distro{{Id: "d1"}, {Id: "d2"}}
	for _, d := range distros {
		err := d.Insert(ctx)
		require.NoError(t, err, "Couldn't insert test distro: %v", err)
	}

	err := projectRef.Insert(t.Context())
	require.NoError(t, err, "Couldn't insert test project ref: %v", err)

	fileBytes, err := os.ReadFile(newProjectPatchFile)
	require.NoError(t, err, "Couldn't read patch file: %v", err)

	// this patch adds a new task to the existing build
	configPatch := &patch.Patch{
		Id:            mgobson.NewObjectId(),
		Project:       patchedProject,
		BuildVariants: []string{"linux-64-duroff"},
		Githash:       patchedRevision,
		Patches: []patch.ModulePatch{
			{
				Githash: "revision",
				PatchSet: patch.PatchSet{
					Patch:   string(fileBytes),
					Summary: []thirdparty.Summary{{Name: newConfigFilePath}},
				},
			},
		},
	}
	err = configPatch.Insert(t.Context())
	require.NoError(t, err, "Couldn't insert test patch: %v", err)
	return configPatch
}

func TestSetPriority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(patch.Collection, task.Collection))
	patches := []*patch.Patch{
		{Id: patch.NewId("aabbccddeeff001122334455"), Version: "aabbccddeeff001122334455"},
	}
	t1 := task.Task{
		Id:      "t1",
		Version: "aabbccddeeff001122334455",
	}
	t2 := task.Task{
		Id:            "t2",
		Version:       "something_else",
		ParentPatchID: t1.Version,
	}
	assert.NoError(t, t1.Insert(t.Context()))
	assert.NoError(t, t2.Insert(t.Context()))
	for _, p := range patches {
		assert.NoError(t, p.Insert(t.Context()))
	}
	err := SetVersionsPriority(ctx, []string{"aabbccddeeff001122334455"}, 7, "")
	assert.NoError(t, err)
	foundTask, err := task.FindOneId(ctx, "t1")
	assert.NoError(t, err)
	assert.Equal(t, int64(7), foundTask.Priority)

	foundTask, err = task.FindOneId(ctx, "t2")
	assert.NoError(t, err)
	assert.Equal(t, int64(7), foundTask.Priority)
}

func TestGetPatchedProjectAndGetPatchedProjectConfig(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testutil.ConfigureIntegrationTest(t, patchTestConfig)
	Convey("With calling GetPatchedProject with a config and remote configuration path",
		t, func() {
			Convey("Calling GetPatchedProject returns a valid project given a patch and settings", func() {
				configPatch := resetPatchSetup(ctx, t, remotePath)
				project, patchConfig, err := GetPatchedProject(ctx, patchTestConfig, configPatch)
				So(err, ShouldBeNil)
				So(project, ShouldNotBeNil)
				So(patchConfig, ShouldNotBeNil)
				So(patchConfig.PatchedParserProjectYAML, ShouldNotBeEmpty)
				So(patchConfig.PatchedParserProject, ShouldNotBeNil)

				Convey("Calling GetPatchedProjectConfig should return the same project config as GetPatchedProject", func() {
					projectConfig, err := GetPatchedProjectConfig(ctx, configPatch)
					So(err, ShouldBeNil)
					So(projectConfig, ShouldEqual, patchConfig.PatchedProjectConfig)
				})

				Convey("Calling GetPatchedProject with a created but unfinalized patch", func() {
					configPatch := resetPatchSetup(ctx, t, remotePath)

					// Simulate what patch creation does.
					patchConfig.PatchedParserProject.Id = configPatch.Id.Hex()
					So(patchConfig.PatchedParserProject.Insert(t.Context()), ShouldBeNil)
					configPatch.ProjectStorageMethod = evergreen.ProjectStorageMethodDB
					configPatch.PatchedProjectConfig = patchConfig.PatchedProjectConfig

					projectFromPatchAndDB, patchConfigFromPatchAndDB, err := GetPatchedProject(ctx, patchTestConfig, configPatch)
					So(err, ShouldBeNil)
					So(projectFromPatchAndDB, ShouldNotBeNil)
					So(len(projectFromPatchAndDB.Tasks), ShouldEqual, len(project.Tasks))
					So(patchConfig, ShouldNotBeNil)
					So(patchConfigFromPatchAndDB.PatchedParserProject, ShouldNotBeNil)
					So(len(patchConfigFromPatchAndDB.PatchedParserProject.Tasks), ShouldEqual, len(patchConfig.PatchedParserProject.Tasks))
					So(patchConfigFromPatchAndDB.PatchedProjectConfig, ShouldEqual, patchConfig.PatchedProjectConfig)

					Convey("Calling GetPatchedProjectConfig should return the same project config as GetPatchedProject", func() {
						projectConfigFromPatch, err := GetPatchedProjectConfig(ctx, configPatch)
						So(err, ShouldBeNil)
						So(projectConfigFromPatch, ShouldEqual, patchConfig.PatchedProjectConfig)
					})
				})
			})

			Convey("Calling GetPatchedProject on a project-less version returns a valid project", func() {
				configPatch := resetProjectlessPatchSetup(ctx, t)
				project, patchConfig, err := GetPatchedProject(ctx, patchTestConfig, configPatch)
				So(err, ShouldBeNil)
				So(patchConfig, ShouldNotBeEmpty)
				So(project, ShouldNotBeNil)

				Convey("Calling GetPatchedProjectConfig should return the same project config as GetPatchedProject", func() {
					projectConfig, err := GetPatchedProjectConfig(ctx, configPatch)
					So(err, ShouldBeNil)
					So(projectConfig, ShouldEqual, patchConfig.PatchedProjectConfig)
				})
			})

			Convey("Calling GetPatchedProject on a patch with GridFS patches works", func() {
				configPatch := resetProjectlessPatchSetup(ctx, t)

				patchFileID := primitive.NewObjectID()
				So(db.WriteGridFile(t.Context(), patch.GridFSPrefix, patchFileID.Hex(), strings.NewReader(configPatch.Patches[0].PatchSet.Patch)), ShouldBeNil)
				configPatch.Patches[0].PatchSet.Patch = ""
				configPatch.Patches[0].PatchSet.PatchFileId = patchFileID.Hex()

				project, patchConfig, err := GetPatchedProject(ctx, patchTestConfig, configPatch)
				So(err, ShouldBeNil)
				So(patchConfig, ShouldNotBeEmpty)
				So(project, ShouldNotBeNil)

				Convey("Calling GetPatchedProjectConfig should return the same project config as GetPatchedProject", func() {
					projectConfig, err := GetPatchedProjectConfig(ctx, configPatch)
					So(err, ShouldBeNil)
					So(projectConfig, ShouldEqual, patchConfig.PatchedProjectConfig)
				})
			})

			Reset(func() {
				So(db.Clear(distro.Collection), ShouldBeNil)
				So(db.ClearGridCollections(patch.GridFSPrefix), ShouldBeNil)
			})
		})
}

func TestFinalizePatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testutil.ConfigureIntegrationTest(t, patchTestConfig)
	require.NoError(t, evergreen.UpdateConfig(ctx, patchTestConfig), ShouldBeNil)

	// Running a multi-document transaction requires the collections to exist
	// first before any documents can be inserted.
	require.NoError(t, db.CreateCollections(manifest.Collection, VersionCollection, ParserProjectCollection, ProjectConfigCollection))

	for name, test := range map[string]func(t *testing.T, p *patch.Patch, patchConfig *PatchConfig){
		"VersionCreationWithParserProjectInDB": func(t *testing.T, p *patch.Patch, patchConfig *PatchConfig) {
			project, patchConfig, err := GetPatchedProject(ctx, patchTestConfig, p)
			require.NoError(t, err)
			assert.NotNil(t, project)

			patchConfig.PatchedParserProject.Id = p.Id.Hex()
			require.NoError(t, patchConfig.PatchedParserProject.Insert(t.Context()))
			ppStorageMethod := evergreen.ProjectStorageMethodDB
			p.ProjectStorageMethod = ppStorageMethod
			require.NoError(t, p.Insert(t.Context()))

			version, err := FinalizePatch(ctx, p, evergreen.PatchVersionRequester)
			require.NoError(t, err)
			assert.NotNil(t, version)
			assert.Len(t, version.Parameters, 1)
			assert.Equal(t, ppStorageMethod, version.ProjectStorageMethod, "version's project storage method should match that of its patch")

			dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
			require.NoError(t, err)
			require.NotZero(t, dbPatch)
			assert.True(t, dbPatch.Activated)
			// ensure the relevant builds/tasks were created
			builds, err := build.Find(t.Context(), build.All)
			require.NoError(t, err)
			assert.Len(t, builds, 1)
			assert.Len(t, builds[0].Tasks, 2)
			tasks, err := task.Find(ctx, bson.M{})
			require.NoError(t, err)
			assert.Len(t, tasks, 2)
		},
		"VersionCreationWithAutoUpdateModules": func(t *testing.T, p *patch.Patch, patchConfig *PatchConfig) {
			modules := ModuleList{
				{
					Name:       "sandbox",
					Branch:     "main",
					Owner:      "evergreen-ci",
					Repo:       "commit-queue-sandbox",
					AutoUpdate: true,
				},
				{
					Name:   "evergreen",
					Branch: "main",
					Owner:  "evergreen-ci",
					Repo:   "evergreen",
				},
			}

			patchConfig.PatchedParserProject.Id = p.Id.Hex()
			patchConfig.PatchedParserProject.Modules = modules
			patchConfig.PatchedParserProject.Identifier = utility.ToStringPtr(p.Project)
			require.NoError(t, patchConfig.PatchedParserProject.Insert(t.Context()))

			p.ProjectStorageMethod = evergreen.ProjectStorageMethodDB
			require.NoError(t, p.Insert(t.Context()))

			project, patchConfig, err := GetPatchedProject(ctx, patchTestConfig, p)
			require.NoError(t, err)
			assert.NotNil(t, project)

			baseManifest := manifest.Manifest{
				Revision:    patchedRevision,
				ProjectName: patchedProject,
				Modules: map[string]*manifest.Module{
					"sandbox":   {Branch: "main", Repo: "sandbox", Owner: "else", Revision: "123"},
					"evergreen": {Branch: "main", Repo: "evergreen", Owner: "something", Revision: "abc"},
				},
				IsBase: true,
			}
			_, err = baseManifest.TryInsert(t.Context())
			require.NoError(t, err)

			version, err := FinalizePatch(ctx, p, evergreen.PatchVersionRequester)
			require.NoError(t, err)
			assert.NotNil(t, version)
			// Ensure that the manifest was created and that auto_update worked for
			// sandbox module but was skipped for evergreen
			mfst, err := manifest.FindOne(ctx, manifest.ById(p.Id.Hex()))
			require.NoError(t, err)
			assert.NotNil(t, mfst)
			assert.Len(t, mfst.Modules, 2)
			assert.NotEqual(t, "123", mfst.Modules["sandbox"].Revision)
			assert.Equal(t, "abc", mfst.Modules["evergreen"].Revision)
		},
		"EmptyCommitQueuePatchDoesntCreateVersion": func(t *testing.T, p *patch.Patch, patchConfig *PatchConfig) {
			patchConfig.PatchedParserProject.Id = p.Id.Hex()
			require.NoError(t, patchConfig.PatchedParserProject.Insert(t.Context()))
			ppStorageMethod := evergreen.ProjectStorageMethodDB
			p.ProjectStorageMethod = ppStorageMethod

			//normal patch should error
			p.Tasks = []string{}
			p.BuildVariants = []string{}
			p.VariantsTasks = []patch.VariantTasks{}
			require.NoError(t, p.Insert(t.Context()))

			_, err := FinalizePatch(ctx, p, evergreen.PatchVersionRequester)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot finalize patch with no tasks")

			// commit queue patch should fail with different error
			p.Alias = evergreen.CommitQueueAlias
			_, err = FinalizePatch(ctx, p, evergreen.GithubMergeRequester)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no builds or tasks for merge queue version")
		},
		"GitHubPRPatchCreatesAllEssentialTasks": func(t *testing.T, p *patch.Patch, patchConfig *PatchConfig) {
			patchConfig.PatchedParserProject.Id = p.Id.Hex()
			require.NoError(t, patchConfig.PatchedParserProject.Insert(t.Context()))
			p.ProjectStorageMethod = evergreen.ProjectStorageMethodDB
			require.NoError(t, p.Insert(t.Context()))

			version, err := FinalizePatch(ctx, p, evergreen.GithubPRRequester)
			require.NoError(t, err)
			assert.NotNil(t, version)
			assert.Len(t, version.Parameters, 1)
			assert.Equal(t, evergreen.ProjectStorageMethodDB, version.ProjectStorageMethod, "version's project storage method should be set")

			dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
			require.NoError(t, err)
			require.NotZero(t, dbPatch)
			assert.True(t, dbPatch.Activated)

			builds, err := build.Find(t.Context(), build.All)
			require.NoError(t, err)
			assert.Len(t, builds, 1)
			assert.Len(t, builds[0].Tasks, 2)

			tasks, err := task.Find(ctx, bson.M{})
			require.NoError(t, err)
			assert.Len(t, tasks, 2)
			for _, tsk := range tasks {
				assert.True(t, tsk.IsEssentialToSucceed, "tasks automatically selected when a GitHub PR patch is finalized should be essential to succeed")
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := resetPatchSetup(ctx, t, remotePath)

			project, patchConfig, err := GetPatchedProject(ctx, patchTestConfig, p)
			require.NoError(t, err)
			assert.NotNil(t, project)

			test(t, p, patchConfig)
		})
	}
}

func TestGetFullPatchParams(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, ProjectAliasCollection, patch.Collection))
	p := patch.Patch{
		Id:      patch.NewId("aaaaaaaaaaff001122334455"),
		Project: "p1",
		Alias:   "test_alias",
		Parameters: []patch.Parameter{
			{
				Key:   "a",
				Value: "3",
			},
			{
				Key:   "c",
				Value: "4",
			},
		},
	}
	alias := ProjectAlias{
		ProjectID: "p1",
		Alias:     "test_alias",
		Variant:   "ubuntu",
		Task:      "subcommand",
		Parameters: []patch.Parameter{
			{
				Key:   "a",
				Value: "1",
			},
			{
				Key:   "b",
				Value: "2",
			},
		},
	}
	pRef := ProjectRef{
		Id: "p1",
	}
	require.NoError(t, pRef.Insert(t.Context()))
	require.NoError(t, p.Insert(t.Context()))
	require.NoError(t, alias.Upsert(t.Context()))

	params, err := getFullPatchParams(t.Context(), &p)
	require.NoError(t, err)
	require.Len(t, params, 3)
	for _, param := range params {
		if param.Key == "a" {
			assert.Equal(t, "3", param.Value)
		}
	}
}

func TestMakePatchedConfig(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	cwd := testutil.GetDirectoryOfFile()

	remoteConfigPath := filepath.Join("config", "evergreen.yml")
	fileBytes, err := os.ReadFile(filepath.Join(cwd, "testdata", "patch.diff"))
	assert.NoError(t, err)
	// update patch with remove config path variable
	diffString := fmt.Sprintf(string(fileBytes),
		remoteConfigPath, remoteConfigPath, remoteConfigPath, remoteConfigPath)
	// the patch adds a new task
	p := &patch.Patch{
		Patches: []patch.ModulePatch{{
			Githash: "revision",
			PatchSet: patch.PatchSet{
				Patch: diffString,
				Summary: []thirdparty.Summary{{
					Name:      remoteConfigPath,
					Additions: 3,
					Deletions: 3,
				}},
			},
		}},
	}
	projectBytes, err := os.ReadFile(filepath.Join(cwd, "testdata", "project.config"))
	assert.NoError(t, err)

	// Test that many goroutines can run MakePatchedConfig at the same time
	wg := sync.WaitGroup{}
	for w := 0; w < 10; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			opts := GetProjectOpts{
				RemotePath: remoteConfigPath,
				PatchOpts: &PatchOpts{
					env:   env,
					patch: p,
				},
			}
			projectData, err := MakePatchedConfig(ctx, opts, string(projectBytes))
			assert.NoError(t, err)
			assert.NotNil(t, projectData)
			project := &Project{}
			_, err = LoadProjectInto(ctx, projectData, nil, "", project)
			assert.NoError(t, err)
			assert.Len(t, project.Tasks, 2)
		}()
	}
	wg.Wait()
}

func TestMakePatchedConfigEmptyBase(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	cwd := testutil.GetDirectoryOfFile()

	remoteConfigPath := filepath.Join("model", "testdata", "project2.config")
	fileBytes, err := os.ReadFile(filepath.Join(cwd, "testdata", "project.diff"))
	assert.NoError(t, err)
	p := &patch.Patch{
		Patches: []patch.ModulePatch{{
			Githash: "revision",
			PatchSet: patch.PatchSet{
				Patch:   string(fileBytes),
				Summary: []thirdparty.Summary{{Name: remoteConfigPath}},
			},
		}},
	}

	opts := GetProjectOpts{
		RemotePath: remoteConfigPath,
		PatchOpts: &PatchOpts{
			env:   env,
			patch: p,
		},
	}
	projectData, err := MakePatchedConfig(ctx, opts, "")
	assert.NoError(t, err)

	project := &Project{}
	_, err = LoadProjectInto(ctx, projectData, nil, "", project)
	assert.NoError(t, err)
	assert.NotNil(t, project)

	require.Len(t, project.Tasks, 1)
	assert.Equal(t, "hello", project.Tasks[0].Name)
}

func TestMakePatchedConfigRenamed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	cwd := testutil.GetDirectoryOfFile()

	// Confirm that renaming a file as part of a patch diff applies without error
	// and returns the expected patched config.
	remoteConfigPath := filepath.Join("config", "evergreen.yml")
	fileBytes, err := os.ReadFile(filepath.Join(cwd, "testdata", "renamed_patch.diff"))
	assert.NoError(t, err)
	diffString := string(fileBytes)
	p := &patch.Patch{
		Patches: []patch.ModulePatch{{
			Githash: "revision",
			PatchSet: patch.PatchSet{
				Patch: diffString,
				Summary: []thirdparty.Summary{{
					Name:      remoteConfigPath,
					Additions: 3,
					Deletions: 3,
				}},
			},
		}},
	}
	projectBytes, err := os.ReadFile(filepath.Join(cwd, "testdata", "include1.yml"))
	assert.NoError(t, err)

	opts := GetProjectOpts{
		Ref:        &ProjectRef{},
		RemotePath: "renamed.yml",
		PatchOpts: &PatchOpts{
			env:   env,
			patch: p,
		},
	}
	projectData, err := MakePatchedConfig(ctx, opts, string(projectBytes))
	assert.NoError(t, err)
	assert.NotNil(t, projectData)

	intermediateProject, err := createIntermediateProject(projectData, false)
	assert.NoError(t, err)
	assert.NotNil(t, intermediateProject)
	require.Len(t, intermediateProject.BuildVariants, 1)
	assert.Equal(t, "Included variant!!!", intermediateProject.BuildVariants[0].DisplayName)
}

func TestParseRenamedOrCopiedFile(t *testing.T) {
	patchContents := `
diff --git a/include2.yml b/copiedInclude.yml
similarity index 100%
copy from include2.yml
copy to copiedInclude.yml

diff --git a/evergreen.yml b/evergreen.yml
index a45dff8..83a8f81 100644
--- a/evergreen.yml
+++ b/evergreen.yml
@@ -5,7 +5,7 @@ stepback: true

 include:
   - filename: include1.yml
-  - filename: include2.yml
+  - filename: rename2.yml
   - filename: include3.yml
   - filename: include4.yml
   - filename: include5.yml
diff --git a/include2.yml b/rename2.yml
similarity index 100%
rename from include2.yml
rename to rename2.yml
`
	renamedFile := parseRenamedOrCopiedFile(patchContents, "rename2.yml")
	assert.Equal(t, "include2.yml", renamedFile)

	renamedFile = parseRenamedOrCopiedFile(patchContents, "copiedInclude.yml")
	assert.Equal(t, "include2.yml", renamedFile)

	patchContents = `
diff --git a/include1.yml b/include1.yml
index 865a6ec..990824f 100644
--- a/include1.yml
+++ b/include1.yml
@@ -1,6 +1,6 @@
 buildvariants:
   - name: generate-tasks-for-version1
-    display_name: "Generate tasks for evergreen version"
+    display_name: "Generate tasks for evergreen version!!"
     batchtime: 0
     activate: true
     run_on:`

	renamedFile = parseRenamedOrCopiedFile(patchContents, "include1.yml")
	assert.Equal(t, "", renamedFile)

	patchContents = `
diff --git a/evergreen.yml b/evergreen.yml
index bd66c6e..b671f40 100644
--- a/evergreen.yml
+++ b/evergreen.yml
@@ -4,7 +4,7 @@ ignore:
 stepback: true

 includes:
-  - include1.yml
+  - renamed.yml

 pre_error_fails_task: true
 pre: &pre
diff --git a/include1.yml b/renamed.yml
similarity index 65%
rename from include1.yml
rename to renamed.yml
index 748e345..7e70f15 100644
--- a/include1.yml
+++ b/renamed.yml
@@ -1,6 +1,6 @@
 buildvariants:
   - name: included-variant
-    display_name: "Included variant"
+    display_name: "Included variant!!!"
     batchtime: 0
     activate: true
     run_on:`

	renamedFile = parseRenamedOrCopiedFile(patchContents, "renamed.yml")
	assert.Equal(t, "include1.yml", renamedFile)
}

// shouldContainPair returns a blank string if its arguments resemble each other, and returns a
// list of pretty-printed diffs between the objects if they do not match.
func shouldContainPair(actual any, expected ...any) string {
	actualPairsList, ok := actual.([]TVPair)

	if !ok {
		return "Assertion requires a list of TVPair objects"
	}

	if len(expected) != 1 {
		return fmt.Sprintf("Assertion requires 1 expected value, you provided %v", len(expected))
	}

	expectedPair, ok := expected[0].(TVPair)
	if !ok {
		return "Assertion requires expected value to be an instance of TVPair"
	}

	for _, ap := range actualPairsList {
		if ap.Variant == expectedPair.Variant && ap.TaskName == expectedPair.TaskName {
			return ""
		}
	}
	return fmt.Sprintf("Expected list to contain pair '%v', but it didn't", expectedPair)
}

func TestVariantTasksToTVPairs(t *testing.T) {
	assert := assert.New(t)

	input := []patch.VariantTasks{
		{
			Variant: "variant",
			Tasks:   []string{"task1", "task2", "task3"},
			DisplayTasks: []patch.DisplayTask{
				{
					Name: "displaytask1",
				},
			},
		},
	}
	output := VariantTasksToTVPairs(input)
	assert.Len(output.ExecTasks, 3)
	assert.Len(output.DisplayTasks, 1)

	original := output.TVPairsToVariantTasks()
	assert.Equal(input, original)
}

func TestAddNewPatch(t *testing.T) {
	assert := assert.New(t)

	require.NoError(t, db.ClearCollections(patch.Collection, VersionCollection, build.Collection, task.Collection, ProjectRefCollection, user.Collection))
	p := &patch.Patch{
		Activated: true,
	}
	u := &user.DBUser{
		Id:                     "test.user",
		NumScheduledPatchTasks: 0,
		LastScheduledTasksAt:   time.Now().Add(-10 * time.Minute),
	}
	v := &Version{
		Id:         "version",
		Revision:   "1234",
		Requester:  evergreen.PatchVersionRequester,
		CreateTime: time.Now(),
		Author:     "Test User",
		AuthorID:   "test.user",
	}
	baseCommitTime := time.Date(2018, time.July, 15, 16, 45, 0, 0, time.UTC)
	baseVersion := &Version{
		Id:         "baseVersion",
		Revision:   "1234",
		Requester:  evergreen.RepotrackerVersionRequester,
		Identifier: "project",
		CreateTime: baseCommitTime,
	}
	assert.NoError(u.Insert(t.Context()))
	assert.NoError(p.Insert(t.Context()))
	assert.NoError(v.Insert(t.Context()))
	assert.NoError(baseVersion.Insert(t.Context()))
	ref := ProjectRef{
		Id:         "project",
		Identifier: "project_name",
		TestSelection: TestSelectionSettings{
			Allowed: utility.TruePtr(),
		},
	}
	assert.NoError(ref.Insert(t.Context()))

	proj := &Project{
		Identifier: "project",
		BuildVariants: []BuildVariant{
			{
				Name:        "variant",
				DisplayName: "My Variant Display",
				Tasks: []BuildVariantTaskUnit{
					{Name: "task1", Variant: "variant"}, {Name: "task2", Variant: "variant"}, {Name: "task3", Variant: "variant"},
				},
				DisplayTasks: []patch.DisplayTask{
					{
						Name:      "displaytask1",
						ExecTasks: []string{"task1", "task2"},
					},
				},
				RunOn: []string{"arch"},
			},
		},
		Tasks: []ProjectTask{
			{Name: "task1"}, {Name: "task2"}, {Name: "task3"},
		},
	}
	tasks := VariantTasksToTVPairs([]patch.VariantTasks{
		{
			Variant: "variant",
			Tasks:   []string{"task1", "task2", "task3"},
			DisplayTasks: []patch.DisplayTask{
				{
					Name: "displaytask1",
				},
			},
		},
	})
	creationInfo := TaskCreationInfo{
		Project:        proj,
		ProjectRef:     &ref,
		Version:        v,
		Pairs:          tasks,
		ActivationInfo: specificActivationInfo{},
		GeneratedBy:    "",
		TestSelection: TestSelectionParams{
			IncludeBuildVariants: []*regexp.Regexp{regexp.MustCompile("variant")},
			IncludeTasks:         []*regexp.Regexp{regexp.MustCompile("task1")},
		},
	}
	_, _, err := addNewBuilds(t.Context(), creationInfo, nil)
	assert.NoError(err)
	dbBuild, err := build.FindOne(t.Context(), db.Q{})
	assert.NoError(err)
	require.NotNil(t, dbBuild)
	assert.Len(dbBuild.Tasks, 2)
	dbVersion, err := VersionFindOne(t.Context(), db.Q{})
	assert.NoError(err)
	require.NotNil(t, dbVersion)
	assert.Len(dbVersion.BuildVariants, 1)
	assert.Equal("variant", dbVersion.BuildVariants[0].BuildVariant)
	assert.Equal("My Variant Display", dbVersion.BuildVariants[0].DisplayName)

	_, _, err = addNewTasksToExistingBuilds(t.Context(), creationInfo, []build.Build{*dbBuild}, "")
	assert.NoError(err)
	dbUser, err := user.FindOneByIdContext(t.Context(), u.Id)
	assert.NoError(err)
	require.NotNil(t, dbUser)
	assert.Equal(4, dbUser.NumScheduledPatchTasks)
	dbTasks, err := task.FindAll(t.Context(), db.Query(task.ByBuildId(dbBuild.Id)))
	assert.NoError(err)
	assert.NotNil(dbBuild)
	require.Len(t, dbTasks, 4)
	assert.Equal("displaytask1", dbTasks[0].DisplayName)
	assert.Equal("task1", dbTasks[1].DisplayName)
	assert.Equal("task2", dbTasks[2].DisplayName)
	assert.Equal("task3", dbTasks[3].DisplayName)
	assert.False(dbTasks[0].TestSelectionEnabled)
	assert.True(dbTasks[1].TestSelectionEnabled)
	assert.True(dbTasks[2].TestSelectionEnabled)
	assert.False(dbTasks[3].TestSelectionEnabled)
	for _, t := range dbTasks {
		if t.DisplayOnly {
			assert.Zero(t.ExecutionPlatform)
		} else {
			assert.Equal(task.ExecutionPlatformHost, t.ExecutionPlatform)
		}
		assert.Equal(t.CreateTime.UTC(), baseCommitTime)
	}
}

func TestAddNewPatchWithMissingBaseVersion(t *testing.T) {
	assert := assert.New(t)

	require.NoError(t, db.ClearCollections(patch.Collection, VersionCollection, build.Collection, task.Collection, ProjectRefCollection))
	p := &patch.Patch{
		Activated: true,
	}
	v := &Version{
		Id:         "version",
		Revision:   "1234",
		Requester:  evergreen.PatchVersionRequester,
		CreateTime: time.Now(),
	}
	assert.NoError(p.Insert(t.Context()))
	assert.NoError(v.Insert(t.Context()))
	ref := ProjectRef{
		Id:         "project",
		Identifier: "project_name",
	}
	assert.NoError(ref.Insert(t.Context()))

	proj := &Project{
		Identifier: "project",
		BuildVariants: []BuildVariant{
			{
				Name: "variant",
				Tasks: []BuildVariantTaskUnit{
					{Name: "task1", Variant: "variant"}, {Name: "task2", Variant: "variant"}, {Name: "task3", Variant: "variant"},
				},
				DisplayTasks: []patch.DisplayTask{
					{
						Name:      "displaytask1",
						ExecTasks: []string{"task1", "task2"},
					},
				},
				RunOn: []string{"arch"},
			},
		},
		Tasks: []ProjectTask{
			{Name: "task1"}, {Name: "task2"}, {Name: "task3"},
		},
	}
	tasks := VariantTasksToTVPairs([]patch.VariantTasks{
		{
			Variant: "variant",
			Tasks:   []string{"task1", "task2", "task3"},
			DisplayTasks: []patch.DisplayTask{
				{
					Name: "displaytask1",
				},
			},
		},
	})
	creationInfo := TaskCreationInfo{
		Project:        proj,
		ProjectRef:     &ref,
		Version:        v,
		Pairs:          tasks,
		ActivationInfo: specificActivationInfo{},
		GeneratedBy:    "",
	}
	_, _, err := addNewBuilds(t.Context(), creationInfo, nil)
	assert.NoError(err)
	dbBuild, err := build.FindOne(t.Context(), db.Q{})
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Len(dbBuild.Tasks, 2)

	_, _, err = addNewTasksToExistingBuilds(t.Context(), creationInfo, []build.Build{*dbBuild}, "")
	assert.NoError(err)
	dbTasks, err := task.FindAll(t.Context(), db.Query(task.ByBuildId(dbBuild.Id)))
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Len(dbTasks, 4)
	assert.Equal("displaytask1", dbTasks[0].DisplayName)
	assert.Equal("task1", dbTasks[1].DisplayName)
	assert.Equal("task2", dbTasks[2].DisplayName)
	assert.Equal("task3", dbTasks[3].DisplayName)
	for _, task := range dbTasks {
		// Dates stored in the DB only have millisecond precision.
		assert.WithinDuration(task.CreateTime, v.CreateTime, time.Millisecond)
	}
}

func TestAddDisplayTasksToPatchReq(t *testing.T) {
	testutil.Setup()
	p := Project{
		BuildVariants: []BuildVariant{
			{
				Name: "bv",
				DisplayTasks: []patch.DisplayTask{
					{Name: "dt1", ExecTasks: []string{"1", "2"}},
					{Name: "dt2", ExecTasks: []string{"3", "4"}},
				}},
		},
	}
	req := PatchUpdate{
		VariantsTasks: []patch.VariantTasks{
			{Variant: "bv", Tasks: []string{"t1", "dt1", "dt2"}},
		},
	}
	addDisplayTasksToPatchReq(&req, p)
	assert.Len(t, req.VariantsTasks[0].Tasks, 1)
	assert.Equal(t, "t1", req.VariantsTasks[0].Tasks[0])
	assert.Len(t, req.VariantsTasks[0].DisplayTasks, 2)
}

func TestAbortPatchesWithGithubPatchData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(patch.Collection, task.Collection, VersionCollection))
	}()
	for tName, tCase := range map[string]func(t *testing.T, p *patch.Patch, v *Version, tsk *task.Task){
		"AbortsGitHubPRPatch": func(t *testing.T, p *patch.Patch, v *Version, tsk *task.Task) {
			require.NoError(t, p.Insert(t.Context()))
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, AbortPatchesWithGithubPatchData(ctx, time.Now(), false, "", p.GithubPatchData.BaseOwner, p.GithubPatchData.BaseRepo, p.GithubPatchData.PRNumber))

			dbTask, err := task.FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.True(t, dbTask.Aborted)
		},
		"IgnoresGitHubPRPatchCreatedAfterTimestamp": func(t *testing.T, p *patch.Patch, v *Version, tsk *task.Task) {
			p.CreateTime = time.Now()
			require.NoError(t, p.Insert(t.Context()))
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, AbortPatchesWithGithubPatchData(ctx, time.Now().Add(-time.Hour), false, "", p.GithubPatchData.BaseOwner, p.GithubPatchData.BaseRepo, p.GithubPatchData.PRNumber))

			dbTask, err := task.FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.False(t, dbTask.Aborted)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(patch.Collection, task.Collection, VersionCollection))
			id := mgobson.NewObjectId()
			v := Version{
				Id:        id.Hex(),
				Status:    evergreen.VersionStarted,
				Activated: utility.TruePtr(),
			}
			require.NoError(t, v.Insert(t.Context()))
			p := patch.Patch{
				Id:         id,
				Version:    v.Id,
				Status:     evergreen.VersionStarted,
				Activated:  true,
				Project:    "project",
				CreateTime: time.Now().Add(-time.Hour),
				GithubPatchData: thirdparty.GithubPatch{
					BaseOwner: "owner",
					BaseRepo:  "repo",
					PRNumber:  12345,
				},
			}
			tsk := task.Task{
				Id:        "task",
				Version:   v.Id,
				Status:    evergreen.TaskStarted,
				Project:   p.Project,
				Activated: true,
			}

			tCase(t, &p, &v, &tsk)
		})
	}
}

func TestConfigurePatch(t *testing.T) {
	defer func() {
		require.NoError(t, db.ClearCollections(patch.Collection, ParserProjectCollection, ProjectRefCollection, VersionCollection, build.Collection, task.Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, p *patch.Patch, v *Version, pRef *ProjectRef){
		"UpdatesJustDescription": func(ctx context.Context, t *testing.T, p *patch.Patch, v *Version, pRef *ProjectRef) {
			require.NoError(t, p.Insert(ctx))

			req := PatchUpdate{
				Description: "updating the description only!",
			}
			_, err := ConfigurePatch(ctx, &evergreen.Settings{}, p, nil, pRef, req)
			assert.NoError(t, err)

			dbPatch, err := patch.FindOneId(ctx, p.Id.Hex())
			assert.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, req.Description, p.Description)
			assert.Len(t, dbPatch.VariantsTasks, 1)
			require.Len(t, dbPatch.VariantsTasks, len(p.VariantsTasks))
			assert.ElementsMatch(t, p.VariantsTasks[0].Tasks, dbPatch.VariantsTasks[0].Tasks)
			assert.Equal(t, p.VariantsTasks[0].Variant, dbPatch.VariantsTasks[0].Variant)
			assert.ElementsMatch(t, dbPatch.Tasks, p.Tasks)
			assert.False(t, p.IsReconfigured)
		},
		"AddsNewTasksToAlreadyFinalizedPatch": func(ctx context.Context, t *testing.T, p *patch.Patch, v *Version, pRef *ProjectRef) {
			p.Activated = true
			p.Version = v.Id
			require.NoError(t, p.Insert(ctx))

			req := PatchUpdate{
				VariantsTasks: []patch.VariantTasks{
					{
						Variant: "my_variant",
						Tasks:   []string{"my_task"},
					},
					{
						Variant: "new_bv",
						Tasks:   []string{"new_task"},
					},
				},
			}
			expectedVarsTasks := map[string]patch.VariantTasks{}
			for _, vt := range req.VariantsTasks {
				expectedVarsTasks[vt.Variant] = vt
			}
			_, err := ConfigurePatch(ctx, &evergreen.Settings{}, p, v, pRef, req)
			assert.NoError(t, err)

			dbPatch, err := patch.FindOneId(ctx, p.Id.Hex())
			assert.NoError(t, err)
			require.NotNil(t, p)
			assert.Len(t, dbPatch.VariantsTasks, len(req.VariantsTasks))
			for _, vt := range dbPatch.VariantsTasks {
				expected, ok := expectedVarsTasks[vt.Variant]
				if !assert.True(t, ok, "expected variant tasks not found for variant '%s'", vt.Variant) {
					continue
				}
				assert.ElementsMatch(t, expected.Tasks, vt.Tasks)
			}
			assert.True(t, p.Activated)
			assert.True(t, p.IsReconfigured)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(patch.Collection, ParserProjectCollection, ProjectRefCollection, VersionCollection, build.Collection, task.Collection))

			ctx := t.Context()
			id := mgobson.NewObjectId()
			buildID := "build_id"
			v := &Version{
				Id:        id.Hex(),
				BuildIds:  []string{buildID},
				Activated: utility.TruePtr(),
			}
			require.NoError(t, v.Insert(ctx))
			p := &patch.Patch{
				Id:                   id,
				Status:               evergreen.VersionCreated,
				Activated:            true,
				Project:              "project",
				ProjectStorageMethod: evergreen.ProjectStorageMethodDB,
				CreateTime:           time.Now().Add(-time.Hour),
				GithubPatchData: thirdparty.GithubPatch{
					BaseOwner: "owner",
					BaseRepo:  "repo",
					PRNumber:  12345,
				},
				Tasks: []string{"my_task"},
				VariantsTasks: []patch.VariantTasks{
					{
						Variant: "my_variant",
						Tasks:   []string{"my_task"},
					},
				},
			}
			pRef := &ProjectRef{
				Id: mgobson.NewObjectId().Hex(),
			}
			require.NoError(t, pRef.Insert(ctx))
			proj := &Project{}

			projYAML := `
tasks:
- name: my_task
- name: new_task

buildvariants:
- name: my_variant
  run_on:
    - distro
  tasks:
    - my_task
- name: new_bv
  run_on:
    - distro
  tasks:
    - new_task
`
			pp, err := LoadProjectInto(ctx, []byte(projYAML), nil, pRef.Id, proj)
			require.NoError(t, err)
			pp.Id = v.Id
			require.NoError(t, pp.Insert(ctx))
			b := &build.Build{
				Id:          buildID,
				DisplayName: "my_variant",
				Version:     v.Id,
				Project:     pRef.Id,
			}
			require.NoError(t, b.Insert(ctx))
			tsk := &task.Task{
				Id:          "task_id",
				DisplayName: "my_task",
				BuildId:     buildID,
				Version:     v.Id,
			}
			require.NoError(t, tsk.Insert(ctx))
			tCase(ctx, t, p, v, pRef)
		})
	}
}
