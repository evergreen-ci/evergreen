package cli

import (
	"io/ioutil"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/yaml.v2"
)

var testConfig = evergreen.TestConfig()
var testPatch = `diff --git a/README.md b/README.md
index e69de29..e5dcf0f 100644
--- a/README.md
+++ b/README.md
@@ -0,0 +1,2 @@
+
+sdgs
`

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
}

func TestCLIFunctions(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestCLIFunctions")

	Convey("with API test server running", t, func() {
		// create a test API server
		testServer, err := apiserver.CreateTestServer(testConfig, nil, plugin.APIPlugins, true)

		// create a test user
		So(db.Clear(user.Collection), ShouldBeNil)
		So(db.Clear(patch.Collection), ShouldBeNil)
		So(db.Clear(model.ProjectRefCollection), ShouldBeNil)
		So((&user.DBUser{Id: "testuser", APIKey: "testapikey", EmailAddress: "tester@mongodb.com"}).Insert(), ShouldBeNil)
		localConfBytes, err := ioutil.ReadFile("testdata/sample.yml")
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
		settings := Settings{
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
		settingsFile.Close()
		t.Log("Wrote settings file to ", settingsFile.Name())

		ac, _, err := getAPIClient(&Options{settingsFile.Name()})
		So(err, ShouldBeNil)

		Convey("check that creating a patch works", func() {
			Convey("user should start with no patches present", func() {
				patches, err := ac.GetPatches(0)
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
					patches, err := ac.GetPatches(0)
					So(err, ShouldBeNil)
					So(len(patches), ShouldEqual, 1)
				})

				Convey("Adding a module to the patch should work", func() {
					err = ac.UpdatePatchModule(newPatch.Id.Hex(), "render-module", testPatch, "1e5232709595db427893826ce19289461cba3f75")
					So(err, ShouldBeNil)
					patches, err := ac.GetPatches(0)
					So(err, ShouldBeNil)
					So(patches[0].Patches[0].ModuleName, ShouldEqual, "")
					So(patches[0].Patches[1].ModuleName, ShouldEqual, "render-module")
					Convey("Removing the module from the patch should work", func() {
						So(ac.DeletePatchModule(newPatch.Id.Hex(), "render-module"), ShouldBeNil)
						patches, err := ac.GetPatches(0)
						So(err, ShouldBeNil)
						So(len(patches[0].Patches), ShouldEqual, 1)
						Convey("Finalizing the patch should work", func() {
							// First double check that the patch starts with no "version" field
							So(patches[0].Version, ShouldEqual, "")
							So(ac.FinalizePatch(newPatch.Id.Hex()), ShouldBeNil)
							patches, err := ac.GetPatches(0)
							So(err, ShouldBeNil)
							// After finalizing, the patch should now have a version populated
							So(patches[0].Version, ShouldNotEqual, "")
							Convey("Cancelling the patch should work", func() {
								So(ac.CancelPatch(newPatch.Id.Hex()), ShouldBeNil)
								patches, err := ac.GetPatches(0)
								So(err, ShouldBeNil)
								// After cancelling, tasks in the version should be deactivated
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
					patches, err := ac.GetPatches(1)
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
							patches, err := ac.GetPatches(1)
							So(err, ShouldBeNil)
							So(len(patches), ShouldEqual, 1)
						})
						Convey("GetPatches where n=2 should return 2 patches", func() {
							patches, err := ac.GetPatches(2)
							So(err, ShouldBeNil)
							So(len(patches), ShouldEqual, 2)
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

		})
	})
}
