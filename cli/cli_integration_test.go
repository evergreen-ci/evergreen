package cli

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
)

var testConfig = testutil.TestConfig()

var testPatch = `diff --git a/README.md b/README.md
index e69de29..e5dcf0f 100644
--- a/README.md
+++ b/README.md
@@ -0,0 +1,2 @@
+
+sdgs
`

var testModulePatch = `
diff --git a/blah.md b/blah.md
new file mode 100644
index 0000000..ce01362
--- /dev/null
+++ b/blah.md
@@ -0,0 +1 @@
+hello
`

var emptyPatch = ``

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
}

type cliTestHarness struct {
	testServer       *service.TestServer
	settingsFilePath string
}

func setupCLITestHarness() cliTestHarness {
	// create a test API server
	testServer, err := service.CreateTestServer(testConfig, nil)
	So(err, ShouldBeNil)
	So(
		db.ClearCollections(
			task.Collection,
			build.Collection,
			user.Collection,
			patch.Collection,
			model.ProjectRefCollection,
			artifact.Collection,
			version.Collection,
		),
		ShouldBeNil)
	So(db.Clear(patch.Collection), ShouldBeNil)
	So(db.Clear(model.ProjectRefCollection), ShouldBeNil)
	So((&user.DBUser{Id: "testuser", APIKey: "testapikey", EmailAddress: "tester@mongodb.com"}).Insert(), ShouldBeNil)
	localConfBytes, err := ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "sample.yml"))
	So(err, ShouldBeNil)

	projectRef := &model.ProjectRef{
		Identifier:  "sample",
		Owner:       "evergreen-ci",
		Repo:        "sample",
		RepoKind:    "github",
		Branch:      "master",
		RemotePath:  "evergreen.yml",
		LocalConfig: string(localConfBytes),
		Enabled:     true,
		BatchTime:   180,
	}
	So(projectRef.Insert(), ShouldBeNil)

	// create a settings file for the command line client
	settings := model.CLISettings{
		APIServerHost: testServer.URL + "/api",
		UIServerHost:  "http://dev-evg.mongodb.com",
		APIKey:        "testapikey",
		User:          "testuser",
	}
	settingsFile, err := ioutil.TempFile("", "settings")
	So(err, ShouldBeNil)
	settingsBytes, err := yaml.Marshal(settings)
	So(err, ShouldBeNil)
	_, err = settingsFile.Write(settingsBytes)
	So(err, ShouldBeNil)
	So(settingsFile.Close(), ShouldBeNil)
	return cliTestHarness{testServer, settingsFile.Name()}
}

func TestCLIFetchSource(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestCLIFetchSource")
	Convey("with a task containing patches and modules", t, func() {
		testSetup := setupCLITestHarness()
		defer testSetup.testServer.Close()
		err := os.RemoveAll("source-patch-1_sample")
		So(err, ShouldBeNil)

		// first, create a patch
		patchSub := patchSubmission{"sample",
			testPatch,
			"sample patch",
			"3c7bfeb82d492dc453e7431be664539c35b5db4b",
			"all",
			[]string{"all"},
			false}

		// Set up a test patch that contains module changes
		ac, rc, _, err := getAPIClients(context.Background(), &Options{testSetup.settingsFilePath})
		So(err, ShouldBeNil)
		newPatch, err := ac.PutPatch(patchSub)
		So(err, ShouldBeNil)
		_, err = ac.GetPatches(0)
		So(err, ShouldBeNil)
		So(ac.UpdatePatchModule(newPatch.Id.Hex(), "render-module", testModulePatch, "1e5232709595db427893826ce19289461cba3f75"),
			ShouldBeNil)
		So(ac.FinalizePatch(newPatch.Id.Hex()), ShouldBeNil)

		patches, err := ac.GetPatches(0)
		So(err, ShouldBeNil)
		testTask, err := task.FindOne(
			db.Query(bson.M{
				task.VersionKey:      patches[0].Version,
				task.BuildVariantKey: "ubuntu",
			}))
		So(err, ShouldBeNil)
		So(testTask, ShouldNotBeNil)

		err = fetchSource(ac, rc, "", testTask.Id, false)
		So(err, ShouldBeNil)

		fileStat, err := os.Stat("./source-patch-1_sample/README.md")
		So(err, ShouldBeNil)
		// If patch was applied correctly, README.md will have a non-zero size
		So(fileStat.Size, ShouldNotEqual, 0)
		// If module was fetched, "render" directory should have been created.
		// The "blah.md" file should have been created if the patch was applied successfully.
		fileStat, err = os.Stat("./source-patch-1_sample/modules/render-module/blah.md")
		So(err, ShouldBeNil)
		So(fileStat.Size, ShouldNotEqual, 0)

	})
}

func TestCLIFetchArtifacts(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestCLIFetchArtifacts")
	Convey("with API test server running", t, func() {
		testSetup := setupCLITestHarness()
		defer testSetup.testServer.Close()

		err := os.RemoveAll("artifacts-abcdef-rest_task_variant_task_one")
		So(err, ShouldBeNil)
		err = os.RemoveAll("artifacts-abcdef-rest_task_variant_task_two")
		So(err, ShouldBeNil)

		err = (&task.Task{
			Id:           "rest_task_test_id1",
			BuildVariant: "rest_task_variant",
			Revision:     "abcdef1234",
			DependsOn:    []task.Dependency{{TaskId: "rest_task_test_id2"}},
			DisplayName:  "task_one",
		}).Insert()
		So(err, ShouldBeNil)

		err = (&task.Task{
			Id:           "rest_task_test_id2",
			Revision:     "abcdef1234",
			BuildVariant: "rest_task_variant",
			DependsOn:    []task.Dependency{},
			DisplayName:  "task_two",
		}).Insert()

		err = (&artifact.Entry{
			TaskId:          "rest_task_test_id1",
			TaskDisplayName: "task_one",
			Files:           []artifact.File{{Link: "http://www.google.com/robots.txt"}},
		}).Upsert()
		So(err, ShouldBeNil)

		err = (&artifact.Entry{
			TaskId:          "rest_task_test_id2",
			TaskDisplayName: "task_two",
			Files:           []artifact.File{{Link: "http://www.google.com/humans.txt"}},
		}).Upsert()
		So(err, ShouldBeNil)

		_, rc, _, err := getAPIClients(context.Background(), &Options{testSetup.settingsFilePath})
		So(err, ShouldBeNil)

		Convey("shallow fetch artifacts should download a single task's artifacts successfully", func() {
			err = fetchArtifacts(rc, "rest_task_test_id1", "", true)
			So(err, ShouldBeNil)
			// downloaded file should exist where we expect
			fileStat, err := os.Stat("./artifacts-abcdef-rest_task_variant_task_one/robots.txt")
			So(err, ShouldBeNil)
			So(fileStat.Size(), ShouldBeGreaterThan, 0)

			fileStat, err = os.Stat("./rest_task_variant_task_two/humans.txt")
			So(os.IsNotExist(err), ShouldBeTrue)
			Convey("deep fetch artifacts should also download artifacts from dependency", func() {
				err = fetchArtifacts(rc, "rest_task_test_id1", "", false)
				So(err, ShouldBeNil)
				fileStat, err = os.Stat("./artifacts-abcdef-rest_task_variant_task_two/humans.txt")
				So(os.IsNotExist(err), ShouldBeFalse)
			})
		})
	})
}

func TestCLITestHistory(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestCLITestHistory")
	Convey("with API test server running", t, func() {
		testSetup := setupCLITestHarness()
		defer testSetup.testServer.Close()

		Convey("with a set of tasks being inserted into the database", func() {
			now := time.Now()
			revisionBeginning := "101112dfac9f1251466afe7c4bf9f56b"
			project := "sample"
			testVersion := version.Version{
				Id:                  "version1",
				Revision:            fmt.Sprintf("%vversion1", revisionBeginning),
				RevisionOrderNumber: 1,
				Identifier:          project,
				Requester:           evergreen.RepotrackerVersionRequester,
			}
			So(testVersion.Insert(), ShouldBeNil)
			testVersion2 := version.Version{
				Id:                  "version2",
				Revision:            fmt.Sprintf("%vversion2", revisionBeginning),
				RevisionOrderNumber: 2,
				Identifier:          project,
				Requester:           evergreen.RepotrackerVersionRequester,
			}
			So(testVersion2.Insert(), ShouldBeNil)
			testVersion3 := version.Version{
				Id:                  "version3",
				Revision:            fmt.Sprintf("%vversion3", revisionBeginning),
				RevisionOrderNumber: 4,
				Identifier:          project,
				Requester:           evergreen.RepotrackerVersionRequester,
			}
			So(testVersion3.Insert(), ShouldBeNil)
			// create tasks with three different display names that start and finish at various times
			for i := 0; i < 10; i++ {
				startTime := now.Add(time.Minute * time.Duration(i))
				endTime := now.Add(time.Minute * time.Duration(i+1))
				passingResult := task.TestResult{
					TestFile:  "passingTest",
					Status:    evergreen.TestSucceededStatus,
					StartTime: float64(startTime.Unix()),
					EndTime:   float64(endTime.Unix()),
				}
				failedResult := task.TestResult{
					TestFile:  "failingTest",
					Status:    evergreen.TestFailedStatus,
					StartTime: float64(startTime.Unix()),
					EndTime:   float64(endTime.Unix()),
				}
				t := task.Task{
					Id:           fmt.Sprintf("task_%v", i),
					Project:      project,
					DisplayName:  fmt.Sprintf("testTask_%v", i%3),
					Revision:     fmt.Sprintf("%vversion%v", revisionBeginning, i%3),
					Version:      fmt.Sprintf("version%v", i%3),
					BuildVariant: "osx",
					Status:       evergreen.TaskFailed,
					TestResults:  []task.TestResult{passingResult, failedResult},
				}
				So(t.Insert(), ShouldBeNil)
			}

			Convey("with a CLI test history command with tasks, executing should set defaults and print out results", func() {
				thc := TestHistoryCommand{
					GlobalOpts: &Options{testSetup.settingsFilePath},
					Project:    project,
					Tasks:      []string{"testTask_1"},
					Limit:      20,
				}
				So(thc.Execute([]string{}), ShouldBeNil)
			})
		})
	})

}

func TestCLIFunctions(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestCLIFunctions")

	var patches []patch.Patch

	Convey("with API test server running", t, func() {
		testSetup := setupCLITestHarness()
		defer testSetup.testServer.Close()

		ac, _, _, err := getAPIClients(context.Background(), &Options{testSetup.settingsFilePath})
		So(err, ShouldBeNil)

		Convey("check that creating a patch works", func() {
			Convey("user should start with no patches present", func() {
				patches, err = ac.GetPatches(0)
				So(err, ShouldBeNil)
				So(len(patches), ShouldEqual, 0)
			})

			Convey("Creating a simple patch should be successful", func() {
				patchSub := patchSubmission{"sample",
					testPatch,
					"sample patch",
					"3c7bfeb82d492dc453e7431be664539c35b5db4b",
					"all",
					[]string{"all"},
					false}

				newPatch, err := ac.PutPatch(patchSub)
				So(err, ShouldBeNil)

				Convey("Newly created patch should be fetchable via API", func() {
					patches, err = ac.GetPatches(0)
					So(err, ShouldBeNil)
					So(len(patches), ShouldEqual, 1)
				})

				Convey("Adding a module to the patch should work", func() {
					err = ac.UpdatePatchModule(newPatch.Id.Hex(), "render-module", testPatch, "1e5232709595db427893826ce19289461cba3f75")
					So(err, ShouldBeNil)
					patches, err = ac.GetPatches(0)
					So(err, ShouldBeNil)
					So(patches[0].Patches[0].ModuleName, ShouldEqual, "")
					So(patches[0].Patches[1].ModuleName, ShouldEqual, "render-module")
					Convey("Removing the module from the patch should work", func() {
						So(ac.DeletePatchModule(newPatch.Id.Hex(), "render-module"), ShouldBeNil)
						patches, err = ac.GetPatches(0)
						So(err, ShouldBeNil)
						So(len(patches[0].Patches), ShouldEqual, 1)
						Convey("Finalizing the patch should work", func() {
							// First double check that the patch starts with no "version" field
							So(patches[0].Version, ShouldEqual, "")
							So(ac.FinalizePatch(newPatch.Id.Hex()), ShouldBeNil)
							patches, err = ac.GetPatches(0)
							So(err, ShouldBeNil)
							// After finalizing, the patch should now have a version populated
							So(patches[0].Version, ShouldNotEqual, "")
							Convey("Canceling the patch should work", func() {
								So(ac.CancelPatch(newPatch.Id.Hex()), ShouldBeNil)
								patches, err = ac.GetPatches(0)
								So(err, ShouldBeNil)
								// After canceling, tasks in the version should be deactivated
								tasks, err := task.Find(task.ByVersion(patches[0].Version))
								So(err, ShouldBeNil)
								for _, t := range tasks {
									So(t.Activated, ShouldBeFalse)
								}
							})
						})
					})
				})
			})

			Convey("Creating a patch without variants should be successful", func() {
				patchSub := patchSubmission{
					"sample",
					testPatch,
					"sample patch",
					"3c7bfeb82d492dc453e7431be664539c35b5db4b",
					"all",
					[]string{},
					false,
				}
				_, err := ac.PutPatch(patchSub)
				So(err, ShouldBeNil)
			})

			Convey("Creating a complex patch should be successful", func() {
				patchSub := patchSubmission{"sample",
					testPatch,
					"sample patch #2",
					"3c7bfeb82d492dc453e7431be664539c35b5db4b",
					"osx-108",
					[]string{"failing_test"},
					false}

				_, err := ac.PutPatch(patchSub)
				So(err, ShouldBeNil)

				Convey("Newly created patch should be fetchable via API", func() {
					patches, err = ac.GetPatches(1)
					So(err, ShouldBeNil)
					So(len(patches), ShouldEqual, 1)
					So(len(patches[0].BuildVariants), ShouldEqual, 1)
					So(patches[0].BuildVariants[0], ShouldEqual, "osx-108")
					So(len(patches[0].Tasks), ShouldEqual, 2)
					So(patches[0].Tasks, ShouldContain, "failing_test")
					Convey("and have expanded dependencies", func() {
						So(patches[0].Tasks, ShouldContain, "compile")
					})

					Convey("putting the patch again", func() {
						_, err := ac.PutPatch(patchSub)
						So(err, ShouldBeNil)
						Convey("GetPatches where n=1 should return 1 patch", func() {
							patches, err = ac.GetPatches(1)
							So(err, ShouldBeNil)
							So(len(patches), ShouldEqual, 1)
						})
						Convey("GetPatches where n=2 should return 2 patches", func() {
							patches, err = ac.GetPatches(2)
							So(err, ShouldBeNil)
							So(len(patches), ShouldEqual, 2)
						})
					})
				})
			})

			Convey("Creating an empty patch should not error out anything", func() {
				patchSub := patchSubmission{
					projectId:   "sample",
					patchData:   emptyPatch,
					description: "sample patch",
					base:        "3c7bfeb82d492dc453e7431be664539c35b5db4b",
					variants:    "all",
					tasks:       []string{"all"},
					finalize:    false}

				newPatch, err := ac.PutPatch(patchSub)
				So(err, ShouldBeNil)

				Convey("Newly created patch should be fetchable via API", func() {
					patches, err = ac.GetPatches(0)
					So(err, ShouldBeNil)
					So(len(patches), ShouldEqual, 1)
				})

				Convey("Adding a module to the patch should still work as designed even with empty patch", func() {
					err = ac.UpdatePatchModule(newPatch.Id.Hex(), "render-module", emptyPatch, "1e5232709595db427893826ce19289461cba3f75")
					So(err, ShouldBeNil)
					patches, err := ac.GetPatches(0)
					So(err, ShouldBeNil)
					So(patches[0].Patches[0].ModuleName, ShouldEqual, "")
					So(patches[0].Patches[1].ModuleName, ShouldEqual, "render-module")
					Convey("Removing the module from the patch should work as designed even with empty patch", func() {
						So(ac.DeletePatchModule(newPatch.Id.Hex(), "render-module"), ShouldBeNil)
						patches, err := ac.GetPatches(0)
						So(err, ShouldBeNil)
						So(len(patches[0].Patches), ShouldEqual, 1)
						Convey("Finalizing the patch should start with no version field and then be populated", func() {
							So(patches[0].Version, ShouldEqual, "")
							So(ac.FinalizePatch(newPatch.Id.Hex()), ShouldBeNil)
							patches, err := ac.GetPatches(0)
							So(err, ShouldBeNil)
							So(patches[0].Version, ShouldNotEqual, "")
							Convey("Canceling the patch should work and the version should be deactivated", func() {
								So(ac.CancelPatch(newPatch.Id.Hex()), ShouldBeNil)
								patches, err := ac.GetPatches(0)
								So(err, ShouldBeNil)
								tasks, err := task.Find(task.ByVersion(patches[0].Version))
								So(err, ShouldBeNil)
								for _, t := range tasks {
									So(t.Activated, ShouldBeFalse)
								}
							})
						})
					})
				})
			})
			Convey("Listing variants or tasks for a project should list all variants", func() {
				tasks, err := ac.ListTasks("sample")
				So(err, ShouldBeNil)
				So(tasks, ShouldNotBeEmpty)
				So(len(tasks), ShouldEqual, 4)
			})
			Convey("Listing variants for a project should list all variants", func() {

				variants, err := ac.ListVariants("sample")
				So(err, ShouldBeNil)
				So(variants, ShouldNotBeEmpty)
				So(len(variants), ShouldEqual, 2)
			})
			Convey("Creating a patch using 'all' as variants should schedule all variants", func() {
				patchSub := patchSubmission{"sample",
					testPatch,
					"sample patch #2",
					"3c7bfeb82d492dc453e7431be664539c35b5db4b",
					"all",
					[]string{"failing_test"},
					false}

				_, err := ac.PutPatch(patchSub)
				So(err, ShouldBeNil)

				Convey("Newly created patch should be fetchable via API", func() {
					patches, err := ac.GetPatches(1)
					So(err, ShouldBeNil)
					So(len(patches), ShouldEqual, 1)
					So(len(patches[0].BuildVariants), ShouldEqual, 2)
					So(patches[0].BuildVariants, ShouldContain, "osx-108")
					So(patches[0].BuildVariants, ShouldContain, "ubuntu")
					So(len(patches[0].Tasks), ShouldEqual, 2)
					So(patches[0].Tasks, ShouldContain, "failing_test")
					Convey("and have expanded dependencies", func() {
						So(patches[0].Tasks, ShouldContain, "compile")
					})
				})
			})

			Convey("Creating a patch using 'all' as tasks should schedule all tasks", func() {
				patchSub := patchSubmission{"sample",
					testPatch,
					"sample patch #2",
					"3c7bfeb82d492dc453e7431be664539c35b5db4b",
					"osx-108",
					[]string{"all"},
					false}

				_, err := ac.PutPatch(patchSub)
				So(err, ShouldBeNil)

				Convey("Newly created patch should be fetchable via API", func() {
					patches, err := ac.GetPatches(1)
					So(err, ShouldBeNil)
					So(len(patches), ShouldEqual, 1)
					So(len(patches[0].BuildVariants), ShouldEqual, 1)
					So(patches[0].BuildVariants[0], ShouldEqual, "osx-108")
					So(len(patches[0].Tasks), ShouldEqual, 4)
					So(patches[0].Tasks, ShouldContain, "compile")
					So(patches[0].Tasks, ShouldContain, "passing_test")
					So(patches[0].Tasks, ShouldContain, "failing_test")
					So(patches[0].Tasks, ShouldContain, "timeout_test")
				})
			})

		})
	})
}
