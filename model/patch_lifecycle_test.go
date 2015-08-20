package model

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"os"
	"testing"
)

var (
	projectConfig        = "testdata/project.config"
	patchFile            = "testdata/patch.diff"
	fullProjectPatchFile = "testdata/project.diff"
)

func TestMakePatchedConfig(t *testing.T) {

	Convey("With calling MakePatchedConfig with a config and remote configuration path", t, func() {
		Convey("the config should be patched correctly", func() {
			remoteConfigPath := "config/evergreen.yml"
			fileBytes, err := ioutil.ReadFile(patchFile)
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
			projectBytes, err := ioutil.ReadFile(projectConfig)
			So(err, ShouldBeNil)
			project, err := MakePatchedConfig(p, remoteConfigPath, string(projectBytes))
			So(err, ShouldBeNil)
			So(project, ShouldNotBeNil)
			So(len(project.Tasks), ShouldEqual, 2)
		})
		Convey("an empty base config should be patched correctly", func() {
			remoteConfigPath := "model/testdata/project2.config"
			fileBytes, err := ioutil.ReadFile(fullProjectPatchFile)
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
			project, err := MakePatchedConfig(p, remoteConfigPath, "")
			So(err, ShouldBeNil)
			So(project, ShouldNotBeNil)
			So(len(project.Tasks), ShouldEqual, 1)
			So(project.Tasks[0].Name, ShouldEqual, "hello")

			Reset(func() {
				os.Remove(remoteConfigPath)
			})

		})
	})
}
