package validator

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
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
)

func init() {
	db.SetGlobalSessionProvider(
		db.SessionFactoryFromConfig(patchTestConfig))
}

func TestFinalize(t *testing.T) {
	testutils.ConfigureIntegrationTest(t, patchTestConfig, "TestFinalize")

	Convey("With calling ValidateAndFinalize with a config and remote configuration "+
		"path", t, func() {
		util.HandleTestingErr(db.ClearCollections(
			model.ProjectRefCollection,
			patch.Collection,
			version.Collection,
			model.BuildsCollection,
			model.TasksCollection),
			t, "Error clearing test collection")

		Convey("a patched config should drive version creation", func() {
			configFilePath := "testing/mci.yml"
			projectRef := &model.ProjectRef{
				Identifier: patchedProject,
				Remote:     true,
				RemotePath: configFilePath,
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
			builds, err := model.FindAllBuilds(bson.M{},
				db.NoProjection,
				db.NoSort,
				db.NoSkip,
				db.NoLimit,
			)
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
				Remote:     true,
				RemotePath: configFilePath,
			}
			err := projectRef.Insert()
			util.HandleTestingErr(err, t, "Couldn't insert test project ref: "+
				"%v", err)
			patchedConfigFile := "fakeInPatchSoNotPatched"
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
			builds, err := model.FindAllBuilds(bson.M{},
				db.NoProjection,
				db.NoSort,
				db.NoSkip,
				db.NoLimit,
			)
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

		Convey("local config patches shouldn't drive version creation", func() {
			projectRef := &model.ProjectRef{
				Identifier: unpatchedProject,
				Remote:     true,
				RemotePath: configFilePath,
			}
			err := projectRef.Insert()
			util.HandleTestingErr(err, t, "Couldn't insert test project ref: "+
				"%v", err)
			fileBytes, err := ioutil.ReadFile(patchFile)
			So(err, ShouldBeNil)
			// this patch adds a new task to the existing build
			configPatch := &patch.Patch{
				Id:            "52549c143122",
				Project:       unpatchedProject,
				BuildVariants: []string{"all"},
				Githash:       unpatchedRevision,
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
			builds, err := model.FindAllBuilds(bson.M{},
				db.NoProjection,
				db.NoSort,
				db.NoSkip,
				db.NoLimit,
			)
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
	})
}
