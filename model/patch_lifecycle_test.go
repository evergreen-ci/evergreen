package model

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/grip"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mgobson "gopkg.in/mgo.v2/bson"
)

var (
	patchTestConfig = testutil.TestConfig()
	configFilePath  = "testing/mci.yml"
	patchedProject  = "mci-config"
	patchedRevision = "3578f10b95fb82183662387048b268c54fac50fb"
	patchFile       = "testdata/patch2.diff"
	patchOwner      = "deafgoat"
	patchRepo       = "config"
	patchBranch     = "master"

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
	require.NoError(t, db.ClearCollections(ProjectRefCollection, patch.Collection, VersionCollection, build.Collection, task.Collection, distro.Collection), "Error clearing test collection")
}

// resetPatchSetup clears the ProjectRef, Patch, Version, Build, and Task Collections
// and creates a patch from the test path given.
func resetPatchSetup(t *testing.T, testPath string) *patch.Patch {
	clearAll(t)
	projectRef := &ProjectRef{
		Id:         patchedProject,
		RemotePath: configFilePath,
		Owner:      patchOwner,
		Repo:       patchRepo,
		Branch:     patchBranch,
	}
	// insert distros to be used
	distros := []distro.Distro{{Id: "d1"}, {Id: "d2"}}
	for _, d := range distros {
		err := d.Insert()
		require.NoError(t, err, "Couldn't insert test distro: %v", err)
	}

	err := projectRef.Insert()
	require.NoError(t, err, "Couldn't insert test project ref: %v", err)

	baseVersion := &Version{
		Identifier: patchedProject,
		CreateTime: time.Now(),
		Revision:   patchedRevision,
		Requester:  evergreen.RepotrackerVersionRequester,
	}
	err = baseVersion.Insert()
	require.NoError(t, err, "Couldn't insert test base version: %v", err)

	fileBytes, err := ioutil.ReadFile(patchFile)
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
						{Name: configFilePath, Additions: 4, Deletions: 80},
						{Name: "random.txt", Additions: 6, Deletions: 0},
					},
				},
			},
		},
	}
	err = configPatch.Insert()
	require.NoError(t, err, "Couldn't insert test patch: %v", err)
	return configPatch
}

func resetProjectlessPatchSetup(t *testing.T) *patch.Patch {
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
		err := d.Insert()
		require.NoError(t, err, "Couldn't insert test distro: %v", err)
	}

	err := projectRef.Insert()
	require.NoError(t, err, "Couldn't insert test project ref: %v", err)

	fileBytes, err := ioutil.ReadFile(newProjectPatchFile)
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
	err = configPatch.Insert()
	require.NoError(t, err, "Couldn't insert test patch: %v", err)
	return configPatch
}

func TestGetPatchedProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testutil.ConfigureIntegrationTest(t, patchTestConfig, "TestConfigurePatch")
	Convey("With calling GetPatchedProject with a config and remote configuration path",
		t, func() {
			Convey("Calling GetPatchedProject returns a valid project given a patch and settings", func() {
				configPatch := resetPatchSetup(t, configFilePath)
				token, err := patchTestConfig.GetGithubOauthToken()
				So(err, ShouldBeNil)
				project, projectYaml, err := GetPatchedProject(ctx, configPatch, token)
				So(err, ShouldBeNil)
				So(projectYaml, ShouldNotBeEmpty)
				So(project, ShouldNotBeNil)
			})

			Convey("Calling GetPatchedProject on a project-less version returns a valid project", func() {
				configPatch := resetProjectlessPatchSetup(t)
				token, err := patchTestConfig.GetGithubOauthToken()
				So(err, ShouldBeNil)
				project, projectYaml, err := GetPatchedProject(ctx, configPatch, token)
				So(err, ShouldBeNil)
				So(projectYaml, ShouldNotBeEmpty)
				So(project, ShouldNotBeNil)
			})

			Convey("Calling GetPatchedProject on a patch with GridFS patches works", func() {
				configPatch := resetProjectlessPatchSetup(t)

				patchFileID := primitive.NewObjectID()
				So(db.WriteGridFile(patch.GridFSPrefix, patchFileID.Hex(), strings.NewReader(configPatch.Patches[0].PatchSet.Patch)), ShouldBeNil)
				configPatch.Patches[0].PatchSet.Patch = ""
				configPatch.Patches[0].PatchSet.PatchFileId = patchFileID.Hex()

				token, err := patchTestConfig.GetGithubOauthToken()
				So(err, ShouldBeNil)
				project, projectYaml, err := GetPatchedProject(ctx, configPatch, token)
				So(err, ShouldBeNil)
				So(projectYaml, ShouldNotBeEmpty)
				So(project, ShouldNotBeNil)
			})

			Reset(func() {
				So(db.Clear(distro.Collection), ShouldBeNil)
				So(db.ClearGridCollections(patch.GridFSPrefix), ShouldBeNil)
			})
		})
}

func TestFinalizePatch(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, patchTestConfig, "TestFinalizePatch")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With FinalizePatch on a project and commit event generated from GetPatchedProject path",
		t, func() {
			configPatch := resetPatchSetup(t, configFilePath)
			Convey("a patched config should drive version creation", func() {
				token, err := patchTestConfig.GetGithubOauthToken()
				So(err, ShouldBeNil)
				project, projectYaml, err := GetPatchedProject(ctx, configPatch, token)
				So(err, ShouldBeNil)
				So(project, ShouldNotBeNil)
				configPatch.PatchedConfig = projectYaml
				token, err = patchTestConfig.GetGithubOauthToken()
				So(err, ShouldBeNil)
				version, err := FinalizePatch(ctx, configPatch, evergreen.PatchVersionRequester, token)
				So(err, ShouldBeNil)
				So(version, ShouldNotBeNil)
				So(version.Parameters, ShouldHaveLength, 1)
				// ensure the relevant builds/tasks were created
				builds, err := build.Find(build.All)
				So(err, ShouldBeNil)
				So(len(builds), ShouldEqual, 1)
				So(len(builds[0].Tasks), ShouldEqual, 2)
				tasks, err := task.Find(task.All)
				So(err, ShouldBeNil)
				So(len(tasks), ShouldEqual, 2)
			})

			Convey("a patch that does not include the remote config should not "+
				"drive version creation", func() {
				patchedConfigFile := "fakeInPatchSoNotPatched"
				configPatch := resetPatchSetup(t, patchedConfigFile)
				token, err := patchTestConfig.GetGithubOauthToken()
				So(err, ShouldBeNil)
				project, projectYaml, err := GetPatchedProject(ctx, configPatch, token)
				So(project, ShouldNotBeNil)
				So(err, ShouldBeNil)
				configPatch.PatchedConfig = projectYaml
				token, err = patchTestConfig.GetGithubOauthToken()
				So(err, ShouldBeNil)
				version, err := FinalizePatch(ctx, configPatch, evergreen.PatchVersionRequester, token)
				So(err, ShouldBeNil)
				So(version, ShouldNotBeNil)
				So(err, ShouldBeNil)
				So(version, ShouldNotBeNil)

				// ensure the relevant builds/tasks were created
				builds, err := build.Find(build.All)
				So(err, ShouldBeNil)
				So(len(builds), ShouldEqual, 1)
				So(len(builds[0].Tasks), ShouldEqual, 1)
				tasks, err := task.Find(task.All)
				So(err, ShouldBeNil)
				So(len(tasks), ShouldEqual, 1)
			})

			Convey("a commit queue patch with no tasks/build variants should not create a version", func() {
				//normal patch works
				token, err := patchTestConfig.GetGithubOauthToken()
				So(err, ShouldBeNil)
				configPatch := resetPatchSetup(t, configFilePath)
				configPatch.Tasks = []string{}
				configPatch.BuildVariants = []string{}
				configPatch.VariantsTasks = []patch.VariantTasks{}
				v, err := FinalizePatch(ctx, configPatch, evergreen.MergeTestRequester, token)
				So(err, ShouldBeNil)
				So(v, ShouldNotBeNil)
				So(v.BuildIds, ShouldBeEmpty)

				// commit queue patch should not
				configPatch.Alias = evergreen.CommitQueueAlias
				_, err = FinalizePatch(ctx, configPatch, evergreen.MergeTestRequester, token)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "No builds or tasks for commit queue version")
			})
			Reset(func() {
				So(db.Clear(distro.Collection), ShouldBeNil)
			})
		})
}

func TestMakePatchedConfig(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()

	Convey("With calling MakePatchedConfig with a config and remote configuration path", t, func() {
		cwd := testutil.GetDirectoryOfFile()

		Convey("the config should be patched correctly", func() {
			remoteConfigPath := filepath.Join("config", "evergreen.yml")
			fileBytes, err := ioutil.ReadFile(filepath.Join(cwd, "testdata", "patch.diff"))
			So(err, ShouldBeNil)
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
			projectBytes, err := ioutil.ReadFile(filepath.Join(cwd, "testdata", "project.config"))
			So(err, ShouldBeNil)
			projectData, err := MakePatchedConfig(ctx, env, p, remoteConfigPath, string(projectBytes))
			So(err, ShouldBeNil)
			So(projectData, ShouldNotBeNil)

			project := &Project{}
			_, err = LoadProjectInto(ctx, projectData, nil, "", project)
			So(err, ShouldBeNil)
			So(len(project.Tasks), ShouldEqual, 2)
		})
		Convey("an empty base config should be patched correctly", func() {
			remoteConfigPath := filepath.Join("model", "testdata", "project2.config")
			fileBytes, err := ioutil.ReadFile(filepath.Join(cwd, "testdata", "project.diff"))
			So(err, ShouldBeNil)
			p := &patch.Patch{
				Patches: []patch.ModulePatch{{
					Githash: "revision",
					PatchSet: patch.PatchSet{
						Patch:   string(fileBytes),
						Summary: []thirdparty.Summary{{Name: remoteConfigPath}},
					},
				}},
			}

			projectData, err := MakePatchedConfig(ctx, env, p, remoteConfigPath, "")
			So(err, ShouldBeNil)

			project := &Project{}
			_, err = LoadProjectInto(ctx, projectData, nil, "", project)
			So(err, ShouldBeNil)
			So(project, ShouldNotBeNil)

			So(len(project.Tasks), ShouldEqual, 1)
			So(project.Tasks[0].Name, ShouldEqual, "hello")

			Reset(func() {
				grip.Warning(os.Remove(remoteConfigPath))
			})
		})
	})
}

// shouldContainPair returns a blank string if its arguments resemble each other, and returns a
// list of pretty-printed diffs between the objects if they do not match.
func shouldContainPair(actual interface{}, expected ...interface{}) string {
	actualPairsList, ok := actual.([]TVPair)

	if !ok {
		return fmt.Sprintf("Assertion requires a list of TVPair objects")
	}

	if len(expected) != 1 {
		return fmt.Sprintf("Assertion requires 1 expected value, you provided %v", len(expected))
	}

	expectedPair, ok := expected[0].(TVPair)
	if !ok {
		return fmt.Sprintf("Assertion requires expected value to be an instance of TVPair")
	}

	for _, ap := range actualPairsList {
		if ap.Variant == expectedPair.Variant && ap.TaskName == expectedPair.TaskName {
			return ""
		}
	}
	return fmt.Sprintf("Expected list to contain pair '%v', but it didn't", expectedPair)
}

func TestIncludeDependencies(t *testing.T) {
	Convey("With a project task config with cross-variant dependencies", t, func() {
		parserProject := &ParserProject{
			Tasks: []parserTask{
				{Name: "t1"},
				{Name: "t2", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "t1"}}}},
				{Name: "t3"},
				{Name: "t4", Patchable: new(bool)},
				{Name: "t5", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "t4"}}}},
			},
			BuildVariants: []parserBV{
				{Name: "v1", Tasks: []parserBVTaskUnit{{Name: "t1"}, {Name: "t2"}}},
				{Name: "v2", Tasks: []parserBVTaskUnit{
					{Name: "t3", DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: "t2", Variant: &variantSelector{StringSelector: "v1"}}},
					}},
					{Name: "t4"},
					{Name: "t5"},
				}},
			},
		}
		p, err := TranslateProject(parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		Convey("a patch against v1/t1 should remain unchanged", func() {
			pairs, _ := IncludeDependencies(p, []TVPair{{"v1", "t1"}}, evergreen.PatchVersionRequester)
			So(len(pairs), ShouldEqual, 1)
			So(pairs[0], ShouldResemble, TVPair{"v1", "t1"})
		})

		Convey("a patch against v1/t2 should add t1", func() {
			pairs, _ := IncludeDependencies(p, []TVPair{{"v1", "t2"}}, evergreen.PatchVersionRequester)
			So(len(pairs), ShouldEqual, 2)
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
		})

		Convey("a patch against v2/t3 should add t1,t2, and v1", func() {
			pairs, _ := IncludeDependencies(p, []TVPair{{"v2", "t3"}}, evergreen.PatchVersionRequester)
			So(len(pairs), ShouldEqual, 3)
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
			So(pairs, shouldContainPair, TVPair{"v2", "t3"})
		})

		Convey("a patch against v2/t5 should be pruned, since its dependency is not patchable", func() {
			pairs, _ := IncludeDependencies(p, []TVPair{{"v2", "t5"}}, evergreen.PatchVersionRequester)
			So(len(pairs), ShouldEqual, 0)

			pairs, _ = IncludeDependencies(p, []TVPair{{"v2", "t5"}}, evergreen.RepotrackerVersionRequester)
			So(len(pairs), ShouldEqual, 2)
		})
	})

	Convey("With a project task config with * selectors", t, func() {
		parserProject := &ParserProject{
			Tasks: []parserTask{
				{Name: "t1"},
				{Name: "t2"},
				{Name: "t3", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: AllDependencies}}}},
				{Name: "t4", DependsOn: parserDependencies{
					{TaskSelector: taskSelector{
						Name: "t3", Variant: &variantSelector{StringSelector: AllVariants},
					}},
				}},
				{Name: "t5", DependsOn: parserDependencies{
					{TaskSelector: taskSelector{
						Name: AllDependencies, Variant: &variantSelector{StringSelector: AllVariants},
					}},
				}},
			},
			BuildVariants: []parserBV{
				{Name: "v1", Tasks: []parserBVTaskUnit{{Name: "t1"}, {Name: "t2"}, {Name: "t3"}}},
				{Name: "v2", Tasks: []parserBVTaskUnit{{Name: "t1"}, {Name: "t2"}, {Name: "t3"}}},
				{Name: "v3", Tasks: []parserBVTaskUnit{{Name: "t4"}}},
				{Name: "v4", Tasks: []parserBVTaskUnit{{Name: "t5"}}},
			},
		}
		p, err := TranslateProject(parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		Convey("a patch against v1/t3 should include t2 and t1", func() {
			pairs, _ := IncludeDependencies(p, []TVPair{{"v1", "t3"}}, evergreen.PatchVersionRequester)
			So(len(pairs), ShouldEqual, 3)
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
			So(pairs, shouldContainPair, TVPair{"v1", "t3"})
		})

		Convey("a patch against v3/t4 should include v1, v2, t3, t2, and t1", func() {
			pairs, _ := IncludeDependencies(p, []TVPair{{"v3", "t4"}}, evergreen.PatchVersionRequester)
			So(len(pairs), ShouldEqual, 7)

			So(pairs, shouldContainPair, TVPair{"v3", "t4"})
			// requires t3 on the other variants
			So(pairs, shouldContainPair, TVPair{"v1", "t3"})
			So(pairs, shouldContainPair, TVPair{"v2", "t3"})

			// t3 requires all the others
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
			So(pairs, shouldContainPair, TVPair{"v2", "t2"})
			So(pairs, shouldContainPair, TVPair{"v2", "t1"})
		})

		Convey("a patch against v4/t5 should include v1, v2, v3, t4, t3, t2, and t1", func() {
			pairs, _ := IncludeDependencies(p, []TVPair{{"v4", "t5"}}, evergreen.PatchVersionRequester)
			So(len(pairs), ShouldEqual, 8)
			So(pairs, shouldContainPair, TVPair{"v4", "t5"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t3"})
			So(pairs, shouldContainPair, TVPair{"v2", "t1"})
			So(pairs, shouldContainPair, TVPair{"v2", "t2"})
			So(pairs, shouldContainPair, TVPair{"v2", "t3"})
			So(pairs, shouldContainPair, TVPair{"v3", "t4"})
		})
	})

	Convey("With a project task config with cyclical requirements", t, func() {
		all := []parserBVTaskUnit{{Name: "1"}, {Name: "2"}, {Name: "3"}}
		parserProject := &ParserProject{
			Tasks: []parserTask{
				{Name: "1", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "2"}}, {TaskSelector: taskSelector{Name: "3"}}}},
				{Name: "2", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "1"}}, {TaskSelector: taskSelector{Name: "3"}}}},
				{Name: "3", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "2"}}, {TaskSelector: taskSelector{Name: "1"}}}},
			},
			BuildVariants: []parserBV{
				{Name: "v1", Tasks: all},
				{Name: "v2", Tasks: all},
			},
		}

		p, err := TranslateProject(parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		Convey("all tasks should be scheduled no matter which is initially added", func() {
			Convey("for '1'", func() {
				pairs, _ := IncludeDependencies(p, []TVPair{{"v1", "1"}}, evergreen.PatchVersionRequester)
				So(len(pairs), ShouldEqual, 3)
				So(pairs, shouldContainPair, TVPair{"v1", "1"})
				So(pairs, shouldContainPair, TVPair{"v1", "2"})
				So(pairs, shouldContainPair, TVPair{"v1", "3"})
			})
			Convey("for '2'", func() {
				pairs, _ := IncludeDependencies(p, []TVPair{{"v1", "2"}, {"v2", "2"}}, evergreen.PatchVersionRequester)
				So(len(pairs), ShouldEqual, 6)
				So(pairs, shouldContainPair, TVPair{"v1", "1"})
				So(pairs, shouldContainPair, TVPair{"v1", "2"})
				So(pairs, shouldContainPair, TVPair{"v1", "3"})
				So(pairs, shouldContainPair, TVPair{"v2", "1"})
				So(pairs, shouldContainPair, TVPair{"v2", "2"})
				So(pairs, shouldContainPair, TVPair{"v2", "3"})
			})
			Convey("for '3'", func() {
				pairs, _ := IncludeDependencies(p, []TVPair{{"v2", "3"}}, evergreen.PatchVersionRequester)
				So(len(pairs), ShouldEqual, 3)
				So(pairs, shouldContainPair, TVPair{"v2", "1"})
				So(pairs, shouldContainPair, TVPair{"v2", "2"})
				So(pairs, shouldContainPair, TVPair{"v2", "3"})
			})
		})
	})
	Convey("With a task that depends on task groups", t, func() {
		parserProject := &ParserProject{
			Tasks: []parserTask{
				{Name: "a", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "*", Variant: &variantSelector{StringSelector: "*"}}}}},
				{Name: "b"},
			},
			TaskGroups: []parserTaskGroup{
				{Name: "task-group", Tasks: []string{"b"}},
			},
			BuildVariants: []parserBV{
				{Name: "variant-with-group", Tasks: []parserBVTaskUnit{{Name: "task-group"}}},
				{Name: "initial-variant", Tasks: []parserBVTaskUnit{{Name: "a"}}},
			},
		}
		p, err := TranslateProject(parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		initDep := TVPair{TaskName: "a", Variant: "initial-variant"}
		pairs, _ := IncludeDependencies(p, []TVPair{initDep}, evergreen.PatchVersionRequester)
		So(pairs, ShouldHaveLength, 2)
		So(initDep, ShouldBeIn, pairs)
	})
}

func TestVariantTasksToTVPairs(t *testing.T) {
	assert := assert.New(t)

	input := []patch.VariantTasks{
		patch.VariantTasks{
			Variant: "variant",
			Tasks:   []string{"task1", "task2", "task3"},
			DisplayTasks: []patch.DisplayTask{
				patch.DisplayTask{
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

	require.NoError(t, db.ClearCollections(patch.Collection, VersionCollection, build.Collection, task.Collection, ProjectRefCollection), "problem clearing collections")
	p := &patch.Patch{
		Activated: true,
	}
	v := &Version{
		Id:         "version",
		Revision:   "1234",
		Requester:  evergreen.PatchVersionRequester,
		CreateTime: time.Now(),
	}
	baseCommitTime := time.Date(2018, time.July, 15, 16, 45, 0, 0, time.UTC)
	baseVersion := &Version{
		Id:         "baseVersion",
		Revision:   "1234",
		Requester:  evergreen.RepotrackerVersionRequester,
		Identifier: "project",
		CreateTime: baseCommitTime,
	}
	assert.NoError(p.Insert())
	assert.NoError(v.Insert())
	assert.NoError(baseVersion.Insert())
	ref := ProjectRef{
		Id:         "project",
		Identifier: "project_name",
	}
	assert.NoError(ref.Insert())

	proj := &Project{
		Identifier: "project",
		BuildVariants: []BuildVariant{
			BuildVariant{
				Name: "variant",
				Tasks: []BuildVariantTaskUnit{
					{Name: "task1"}, {Name: "task2"}, {Name: "task3"},
				},
				DisplayTasks: []patch.DisplayTask{
					patch.DisplayTask{
						Name:      "displaytask1",
						ExecTasks: []string{"task1", "task2"},
					},
				},
			},
		},
		Tasks: []ProjectTask{
			ProjectTask{Name: "task1"}, ProjectTask{Name: "task2"}, ProjectTask{Name: "task3"},
		},
	}
	tasks := VariantTasksToTVPairs([]patch.VariantTasks{
		patch.VariantTasks{
			Variant: "variant",
			Tasks:   []string{"task1", "task2", "task3"},
			DisplayTasks: []patch.DisplayTask{
				patch.DisplayTask{
					Name: "displaytask1",
				},
			},
		},
	})

	assert.NoError(AddNewBuildsForPatch(context.Background(), p, v, proj, tasks, &ref))
	dbBuild, err := build.FindOne(db.Q{})
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Len(dbBuild.Tasks, 2)

	assert.NoError(AddNewTasksForPatch(context.Background(), p, v, proj, tasks, ref.Identifier))
	dbTasks, err := task.FindAll(task.ByBuildId(dbBuild.Id))
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Len(dbTasks, 4)
	assert.Equal(dbTasks[0].DisplayName, "displaytask1")
	assert.Equal(dbTasks[1].DisplayName, "task1")
	assert.Equal(dbTasks[2].DisplayName, "task2")
	assert.Equal(dbTasks[3].DisplayName, "task3")
	for _, task := range dbTasks {
		assert.Equal(task.CreateTime.UTC(), baseCommitTime)
	}
}

func TestAddNewPatchWithMissingBaseVersion(t *testing.T) {
	assert := assert.New(t)

	require.NoError(t, db.ClearCollections(patch.Collection, VersionCollection, build.Collection, task.Collection, ProjectRefCollection), "problem clearing collections")
	p := &patch.Patch{
		Activated: true,
	}
	v := &Version{
		Id:         "version",
		Revision:   "1234",
		Requester:  evergreen.PatchVersionRequester,
		CreateTime: time.Now(),
	}
	assert.NoError(p.Insert())
	assert.NoError(v.Insert())
	ref := ProjectRef{
		Id:         "project",
		Identifier: "project_name",
	}
	assert.NoError(ref.Insert())

	proj := &Project{
		Identifier: "project",
		BuildVariants: []BuildVariant{
			BuildVariant{
				Name: "variant",
				Tasks: []BuildVariantTaskUnit{
					{Name: "task1"}, {Name: "task2"}, {Name: "task3"},
				},
				DisplayTasks: []patch.DisplayTask{
					patch.DisplayTask{
						Name:      "displaytask1",
						ExecTasks: []string{"task1", "task2"},
					},
				},
			},
		},
		Tasks: []ProjectTask{
			ProjectTask{Name: "task1"}, ProjectTask{Name: "task2"}, ProjectTask{Name: "task3"},
		},
	}
	tasks := VariantTasksToTVPairs([]patch.VariantTasks{
		patch.VariantTasks{
			Variant: "variant",
			Tasks:   []string{"task1", "task2", "task3"},
			DisplayTasks: []patch.DisplayTask{
				patch.DisplayTask{
					Name: "displaytask1",
				},
			},
		},
	})

	assert.NoError(AddNewBuildsForPatch(context.Background(), p, v, proj, tasks, &ref))
	dbBuild, err := build.FindOne(db.Q{})
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Len(dbBuild.Tasks, 2)

	assert.NoError(AddNewTasksForPatch(context.Background(), p, v, proj, tasks, ref.Identifier))
	dbTasks, err := task.FindAll(task.ByBuildId(dbBuild.Id))
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Len(dbTasks, 4)
	assert.Equal(dbTasks[0].DisplayName, "displaytask1")
	assert.Equal(dbTasks[1].DisplayName, "task1")
	assert.Equal(dbTasks[2].DisplayName, "task2")
	assert.Equal(dbTasks[3].DisplayName, "task3")
	for _, task := range dbTasks {
		// Dates stored in the DB only have millisecond precision.
		assert.WithinDuration(task.CreateTime, v.CreateTime, time.Millisecond)
	}
}

func TestMakeCommitQueueDescription(t *testing.T) {
	projectRef := &ProjectRef{
		Repo:   "evergreen",
		Owner:  "evergreen-ci",
		Branch: "main",
	}

	project := &Project{
		Modules: ModuleList{
			{
				Name:   "module",
				Branch: "feature",
				Repo:   "git@github.com:evergreen-ci/module_repo.git",
			},
		},
	}

	// no commits
	patches := []patch.ModulePatch{}
	assert.Equal(t, "Commit Queue Merge: No Commits Added", MakeCommitQueueDescription(patches, projectRef, project))

	// main repo commit
	patches = []patch.ModulePatch{
		{
			ModuleName: "",
			PatchSet:   patch.PatchSet{CommitMessages: []string{"Commit"}},
		},
	}
	assert.Equal(t, "Commit Queue Merge: 'Commit' into 'evergreen-ci/evergreen:main'", MakeCommitQueueDescription(patches, projectRef, project))

	// main repo + module commits
	patches = []patch.ModulePatch{
		{
			ModuleName: "",
			PatchSet:   patch.PatchSet{CommitMessages: []string{"Commit 1", "Commit 2"}},
		},
		{
			ModuleName: "module",
			PatchSet:   patch.PatchSet{CommitMessages: []string{"Module Commit 1", "Module Commit 2"}},
		},
	}

	assert.Equal(t, "Commit Queue Merge: 'Commit 1 <- Commit 2' into 'evergreen-ci/evergreen:main' || 'Module Commit 1 <- Module Commit 2' into 'evergreen-ci/module_repo:feature'", MakeCommitQueueDescription(patches, projectRef, project))

	// module only commits
	patches = []patch.ModulePatch{
		{
			ModuleName: "",
		},
		{
			ModuleName: "module",
			PatchSet:   patch.PatchSet{CommitMessages: []string{"Module Commit 1", "Module Commit 2"}},
		},
	}
	assert.Equal(t, "Commit Queue Merge: 'Module Commit 1 <- Module Commit 2' into 'evergreen-ci/module_repo:feature'", MakeCommitQueueDescription(patches, projectRef, project))
}

func TestRetryCommitQueueItems(t *testing.T) {
	projectRef := &ProjectRef{
		Id:         patchedProject,
		RemotePath: configFilePath,
		Owner:      patchOwner,
		Repo:       patchRepo,
		Branch:     patchBranch,
	}

	startTime := time.Date(2019, 7, 15, 12, 0, 0, 0, time.Local)
	endTime := startTime.Add(2 * time.Hour)

	opts := RestartOptions{
		StartTime: startTime,
		EndTime:   endTime,
	}

	for name, test := range map[string]func(*testing.T){
		"StartedInRange": func(*testing.T) {
			assert.NoError(t, projectRef.Insert())

			u := user.DBUser{Id: "me", PatchNumber: 12}
			assert.NoError(t, u.Insert())

			// this should just restart the patch with patch #=1
			restarted, notRestarted, err := RetryCommitQueueItems(projectRef.Id, opts)
			assert.NoError(t, err)
			assert.Len(t, restarted, 1)
			assert.Len(t, notRestarted, 0)

			cq, err := commitqueue.FindOneId(projectRef.Id)
			assert.NoError(t, err)
			require.NotNil(t, cq)
			require.Len(t, cq.Queue, 1)
			assert.Equal(t, "123", cq.Queue[0].Issue)
		},
		"UnstartedPatch": func(*testing.T) {
			assert.NoError(t, projectRef.Insert())

			// not started but terminated within time range
			p := patch.Patch{
				Id:         mgobson.NewObjectId(),
				Project:    projectRef.Id,
				Githash:    patchedRevision,
				StartTime:  time.Time{},
				FinishTime: startTime.Add(30 * time.Minute),
				Status:     evergreen.PatchFailed,
				Alias:      evergreen.CommitQueueAlias,
				GithubPatchData: thirdparty.GithubPatch{
					PRNumber: 456,
				},
			}
			assert.NoError(t, p.Insert())
			restarted, notRestarted, err := RetryCommitQueueItems(projectRef.Id, opts)
			assert.NoError(t, err)
			assert.Len(t, restarted, 2)
			assert.Len(t, notRestarted, 0)
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(ProjectRefCollection, commitqueue.Collection, patch.Collection, user.Collection))
			cq := &commitqueue.CommitQueue{ProjectID: projectRef.Id}
			assert.NoError(t, commitqueue.InsertQueue(cq))

			patches := []patch.Patch{
				{ // patch: within time frame, failed
					Id:          mgobson.NewObjectId(),
					PatchNumber: 1,
					Project:     projectRef.Id,
					Githash:     patchedRevision,
					StartTime:   startTime.Add(30 * time.Minute),
					FinishTime:  endTime.Add(30 * time.Minute),
					Status:      evergreen.PatchFailed,
					Alias:       evergreen.CommitQueueAlias,
					Author:      "me",
					GithubPatchData: thirdparty.GithubPatch{
						PRNumber: 123,
					},
					Patches: []patch.ModulePatch{
						{
							Githash:    "revision",
							ModuleName: "name",
							PatchSet: patch.PatchSet{
								Patch: "456",
								Summary: []thirdparty.Summary{
									{Name: configFilePath, Additions: 4, Deletions: 80},
									{Name: "random.txt", Additions: 6, Deletions: 0},
								},
							},
						},
					},
				},
				{ // within time frame, not failed
					Id:          mgobson.NewObjectId(),
					PatchNumber: 2,
					Project:     projectRef.Id,
					Githash:     patchedRevision,
					StartTime:   startTime.Add(30 * time.Minute),
					FinishTime:  endTime.Add(30 * time.Minute),
					Status:      evergreen.PatchSucceeded,
					Alias:       evergreen.CommitQueueAlias,
				},
				{ // within time frame, not commit queue
					Id:          mgobson.NewObjectId(),
					PatchNumber: 3,
					Project:     projectRef.Id,
					Githash:     patchedRevision,
					StartTime:   startTime.Add(30 * time.Minute),
					FinishTime:  endTime.Add(30 * time.Minute),
					Status:      evergreen.PatchFailed,
				},
				{ // not within time frame
					Id:          mgobson.NewObjectId(),
					PatchNumber: 4,
					Project:     projectRef.Id,
					Githash:     patchedRevision,
					StartTime:   time.Date(2019, 6, 15, 12, 0, 0, 0, time.Local),
					FinishTime:  time.Date(2019, 6, 15, 12, 20, 0, 0, time.Local),
					Status:      evergreen.PatchFailed,
					Alias:       evergreen.CommitQueueAlias,
				},
			}
			for _, p := range patches {
				assert.NoError(t, p.Insert())
			}
			test(t)
		})
	}
}
