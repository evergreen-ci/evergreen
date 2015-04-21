package validator

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/model/build"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/patch"
	"10gen.com/mci/model/version"
	"10gen.com/mci/testutils"
	"10gen.com/mci/thirdparty"
	"10gen.com/mci/util"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"testing"
)

var (
	patchTestConfig   = mci.TestConfig()
	configFilePath    = "testing/mci.yml"
	patchedProject    = "mci-config"
	unpatchedProject  = "mci-test"
	patchedRevision   = "582257a4ca3a9c890959b04d4dd2de5e4d34e9e7"
	unpatchedRevision = "99162ee5bc41eb314f5bb01bd12f0c43e9cb5f32"
	patchFile         = "testdata/patch.diff"
	patchOwner        = "deafgoat"
	patchRepo         = "config"
	patchBranch       = "master"
)

func init() {
	db.SetGlobalSessionProvider(
		db.SessionFactoryFromConfig(patchTestConfig))
}

func TestProjectRef(t *testing.T) {
	Convey("When inserting a project ref", t, func() {
		err := testutils.CreateTestLocalConfig(patchTestConfig, "mci-test")
		So(err, ShouldBeNil)

		projectRef, err := model.FindOneProjectRef("mci-test")
		So(err, ShouldBeNil)
		So(projectRef.Identifier, ShouldEqual, "mci-test")
	})
}

func TestFinalize(t *testing.T) {
	testutils.ConfigureIntegrationTest(t, patchTestConfig, "TestFinalize")

	Convey("With calling ValidateAndFinalize with a config and remote configuration "+
		"path", t, func() {
		util.HandleTestingErr(db.ClearCollections(
			model.ProjectRefCollection,
			patch.Collection,
			version.Collection,
			build.Collection,
			model.TasksCollection),
			t, "Error clearing test collection")

		Convey("a patched config should drive version creation", func() {
			configFilePath := "testing/mci.yml"
			// insert distros to be used
			distros := []distro.Distro{
				distro.Distro{Id: "d1"},
				distro.Distro{Id: "d2"},
			}

			for _, d := range distros {
				So(d.Insert(), ShouldBeNil)
			}

			projectRef := &model.ProjectRef{
				Identifier: patchedProject,
				RemotePath: configFilePath,
				Owner:      patchOwner,
				Repo:       patchRepo,
				Branch:     patchBranch,
			}
			err := projectRef.Insert()
			util.HandleTestingErr(err, t, "Couldn't insert test project ref: "+
				"%v", err)
			fileBytes, err := ioutil.ReadFile(patchFile)
			So(err, ShouldBeNil)

			// this patch adds a new task to the existing build
			configPatch := &patch.Patch{
				Id:            "52549c143122",
				Project:       patchedProject,
				BuildVariants: []string{"all"},
				Githash:       patchedRevision,
				Patches: []patch.ModulePatch{
					patch.ModulePatch{
						Githash: "revision",
						PatchSet: patch.PatchSet{
							Patch: fmt.Sprintf(string(fileBytes), configFilePath,
								configFilePath, configFilePath, configFilePath),
							Summary: []thirdparty.Summary{
								thirdparty.Summary{
									Name:      configFilePath,
									Additions: 4,
									Deletions: 80,
								},
								thirdparty.Summary{
									Name:      "random.txt",
									Additions: 6,
									Deletions: 0,
								},
							},
						},
					},
				},
			}
			err = configPatch.Insert()
			util.HandleTestingErr(err, t, "Couldn't insert test patch: %v", err)
			version, err := ValidateAndFinalize(configPatch, patchTestConfig)
			So(err, ShouldBeNil)
			So(version, ShouldNotBeNil)
			// ensure the relevant builds/tasks were created
			builds, err := build.Find(build.All)
			So(err, ShouldBeNil)
			So(len(builds), ShouldEqual, 1)
			So(len(builds[0].Tasks), ShouldEqual, 2)
			tasks, err := model.FindAllTasks(bson.M{},
				db.NoProjection,
				db.NoSort,
				db.NoSkip,
				db.NoLimit,
			)
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 2)
		})

		Convey("a patch that does not include the remote config should not "+
			"drive version creation", func() {
			configFilePath := "testing/mci.yml"
			projectRef := &model.ProjectRef{
				Identifier: patchedProject,
				RemotePath: configFilePath,
				Owner:      patchOwner,
				Repo:       patchRepo,
				Branch:     patchBranch,
			}
			err := projectRef.Insert()
			util.HandleTestingErr(err, t, "Couldn't insert test project ref: "+
				"%v", err)
			patchedConfigFile := "fakeInPatchSoNotPatched"
			fileBytes, err := ioutil.ReadFile(patchFile)
			So(err, ShouldBeNil)

			// insert distros to be used
			distros := []distro.Distro{
				distro.Distro{Id: "d1"},
				distro.Distro{Id: "d2"},
			}

			for _, d := range distros {
				So(d.Insert(), ShouldBeNil)
			}

			// this patch adds a new task to the existing build
			configPatch := &patch.Patch{
				Id:            "52549c143122",
				Project:       patchedProject,
				BuildVariants: []string{"all"},
				Githash:       patchedRevision,
				Patches: []patch.ModulePatch{
					patch.ModulePatch{
						Githash: "revision",
						PatchSet: patch.PatchSet{
							Patch: fmt.Sprintf(string(fileBytes), patchedConfigFile,
								patchedConfigFile, patchedConfigFile, patchedConfigFile),
							Summary: []thirdparty.Summary{
								thirdparty.Summary{
									Name:      configFilePath,
									Additions: 4,
									Deletions: 80,
								},
								thirdparty.Summary{
									Name:      patchedProject,
									Additions: 6,
									Deletions: 0,
								},
							},
						},
					},
				},
			}
			err = configPatch.Insert()
			util.HandleTestingErr(err, t, "Couldn't insert test patch: %v", err)
			version, err := ValidateAndFinalize(configPatch, patchTestConfig)
			So(err, ShouldBeNil)
			So(version, ShouldNotBeNil)

			// ensure the relevant builds/tasks were created
			builds, err := build.Find(build.All)
			So(err, ShouldBeNil)
			So(len(builds), ShouldEqual, 1)
			So(len(builds[0].Tasks), ShouldEqual, 1)
			tasks, err := model.FindAllTasks(bson.M{},
				db.NoProjection,
				db.NoSort,
				db.NoSkip,
				db.NoLimit,
			)
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 1)
		})

		Reset(func() {
			db.Clear(distro.Collection)
		})
	})
}
