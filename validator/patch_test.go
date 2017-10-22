package validator

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/yaml.v2"
)

var (
	patchTestConfig = testutil.TestConfig()
	configFilePath  = "testing/mci.yml"
	patchedProject  = "mci-config"
	patchedRevision = "582257a4ca3a9c890959b04d4dd2de5e4d34e9e7"
	patchFile       = "testdata/patch.diff"
	patchOwner      = "deafgoat"
	patchRepo       = "config"
	patchBranch     = "master"

	// newProjectPatchFile is a diff that adds a new project configuration file
	// located at newConfigFilePath.
	newProjectPatchFile = "testdata/project.diff"
	newConfigFilePath   = "testing/project2.config"
)

func init() {
	current := testutil.GetDirectoryOfFile()
	patchFile = filepath.Join(current, patchFile)
	newProjectPatchFile = filepath.Join(current, newProjectPatchFile)
}

func clearAll(t *testing.T) {
	testutil.HandleTestingErr(
		db.ClearCollections(
			model.ProjectRefCollection,
			patch.Collection,
			version.Collection,
			build.Collection,
			task.Collection,
			distro.Collection,
		), t, "Error clearing test collection: %v")
}

// resetPatchSetup clears the ProjectRef, Patch, Version, Build, and Task Collections
// and creates a patch from the test path given.
func resetPatchSetup(t *testing.T, testPath string) *patch.Patch {
	clearAll(t)
	projectRef := &model.ProjectRef{
		Identifier: patchedProject,
		RemotePath: configFilePath,
		Owner:      patchOwner,
		Repo:       patchRepo,
		Branch:     patchBranch,
	}
	// insert distros to be used
	distros := []distro.Distro{{Id: "d1"}, {Id: "d2"}}
	for _, d := range distros {
		err := d.Insert()
		testutil.HandleTestingErr(err, t, "Couldn't insert test distro: %v", err)
	}

	err := projectRef.Insert()
	testutil.HandleTestingErr(err, t, "Couldn't insert test project ref: %v", err)

	fileBytes, err := ioutil.ReadFile(patchFile)
	testutil.HandleTestingErr(err, t, "Couldn't read patch file: %v", err)

	// this patch adds a new task to the existing build
	configPatch := &patch.Patch{
		Id:            "52549c143122",
		Project:       patchedProject,
		Githash:       patchedRevision,
		Tasks:         []string{"taskTwo", "taskOne"},
		BuildVariants: []string{"linux-64-duroff"},
		Patches: []patch.ModulePatch{
			{
				Githash: "revision",
				PatchSet: patch.PatchSet{
					Patch: fmt.Sprintf(string(fileBytes), testPath, testPath, testPath, testPath),
					Summary: []patch.Summary{
						{Name: configFilePath, Additions: 4, Deletions: 80},
						{Name: "random.txt", Additions: 6, Deletions: 0},
					},
				},
			},
		},
	}
	err = configPatch.Insert()
	testutil.HandleTestingErr(err, t, "Couldn't insert test patch: %v", err)
	return configPatch
}

func resetProjectlessPatchSetup(t *testing.T) *patch.Patch {
	clearAll(t)
	projectRef := &model.ProjectRef{
		Identifier: patchedProject,
		RemotePath: newConfigFilePath,
		Owner:      patchOwner,
		Repo:       patchRepo,
		Branch:     patchBranch,
	}
	// insert distros to be used
	distros := []distro.Distro{{Id: "d1"}, {Id: "d2"}}
	for _, d := range distros {
		err := d.Insert()
		testutil.HandleTestingErr(err, t, "Couldn't insert test distro: %v", err)
	}

	err := projectRef.Insert()
	testutil.HandleTestingErr(err, t, "Couldn't insert test project ref: %v", err)

	fileBytes, err := ioutil.ReadFile(newProjectPatchFile)
	testutil.HandleTestingErr(err, t, "Couldn't read patch file: %v", err)

	// this patch adds a new task to the existing build
	configPatch := &patch.Patch{
		Id:            "52549c143123",
		Project:       patchedProject,
		BuildVariants: []string{"linux-64-duroff"},
		Githash:       patchedRevision,
		Patches: []patch.ModulePatch{
			{
				Githash: "revision",
				PatchSet: patch.PatchSet{
					Patch:   string(fileBytes),
					Summary: []patch.Summary{{Name: newConfigFilePath}},
				},
			},
		},
	}
	err = configPatch.Insert()
	testutil.HandleTestingErr(err, t, "Couldn't insert test patch: %v", err)
	return configPatch
}

func TestProjectRef(t *testing.T) {
	Convey("When inserting a project ref", t, func() {
		err := modelutil.CreateTestLocalConfig(patchTestConfig, "mci-test", "")
		So(err, ShouldBeNil)
		projectRef, err := model.FindOneProjectRef("mci-test")
		So(err, ShouldBeNil)
		So(projectRef, ShouldNotBeNil)
		So(projectRef.Identifier, ShouldEqual, "mci-test")
	})
}

func TestGetPatchedProject(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, patchTestConfig, "TestConfigurePatch")
	Convey("With calling GetPatchedProject with a config and remote configuration path",
		t, func() {
			Convey("Calling GetPatchedProject returns a valid project given a patch and settings", func() {
				configPatch := resetPatchSetup(t, configFilePath)
				project, err := GetPatchedProject(configPatch, patchTestConfig)
				So(err, ShouldBeNil)
				So(project, ShouldNotBeNil)
			})

			Convey("Calling GetPatchedProject on a project-less version returns a valid project", func() {
				configPatch := resetProjectlessPatchSetup(t)
				project, err := GetPatchedProject(configPatch, patchTestConfig)
				So(err, ShouldBeNil)
				So(project, ShouldNotBeNil)
			})

			Reset(func() {
				So(db.Clear(distro.Collection), ShouldBeNil)
			})
		})
}

func TestFinalizePatch(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, patchTestConfig, "TestFinalizePatch")

	Convey("With FinalizePatch on a project and commit event generated from GetPatchedProject path",
		t, func() {
			configPatch := resetPatchSetup(t, configFilePath)
			Convey("a patched config should drive version creation", func() {
				project, err := GetPatchedProject(configPatch, patchTestConfig)
				So(err, ShouldBeNil)
				yamlBytes, err := yaml.Marshal(project)
				So(err, ShouldBeNil)
				configPatch.PatchedConfig = string(yamlBytes)
				version, err := model.FinalizePatch(configPatch, patchTestConfig)
				So(err, ShouldBeNil)
				So(version, ShouldNotBeNil)
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
				project, err := GetPatchedProject(configPatch, patchTestConfig)
				So(err, ShouldBeNil)
				yamlBytes, err := yaml.Marshal(project)
				So(err, ShouldBeNil)
				configPatch.PatchedConfig = string(yamlBytes)
				version, err := model.FinalizePatch(configPatch, patchTestConfig)
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

			Reset(func() {
				So(db.Clear(distro.Collection), ShouldBeNil)
			})
		})
}
