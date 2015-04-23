package model

import (
	"10gen.com/mci/thirdparty"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"testing"
)

var (
	projectConfig = "testdata/project.config"
	patchFile     = "testdata/patch.diff"
)

func TestPatchConfig(t *testing.T) {

	Convey("With calling PatchConfig with a config and remote configuration "+
		"path", t, func() {

		Convey("the config should be patched correctly", func() {
			remoteConfigPath := "config/mci.yml"
			fileBytes, err := ioutil.ReadFile(patchFile)
			So(err, ShouldBeNil)
			// the patch adds a new task
			patch := &Patch{
				Patches: []ModulePatch{
					ModulePatch{
						Githash: "revision",
						PatchSet: PatchSet{
							Patch: fmt.Sprintf(string(fileBytes),
								remoteConfigPath, remoteConfigPath,
								remoteConfigPath, remoteConfigPath),
							Summary: []thirdparty.Summary{
								thirdparty.Summary{
									Name:      remoteConfigPath,
									Additions: 3,
									Deletions: 3,
								},
							},
						},
					},
				},
			}
			projectBytes, err := ioutil.ReadFile(projectConfig)
			So(err, ShouldBeNil)
			project, err := patch.PatchConfig(remoteConfigPath, string(projectBytes))
			So(err, ShouldBeNil)
			So(project, ShouldNotBeNil)
			So(len(project.Tasks), ShouldEqual, 2)
		})
	})
}

func TestConfigChanged(t *testing.T) {

	Convey("With calling ConfigChanged with a remote configuration "+
		"path", t, func() {

		Convey("the function should return true if the config has changed", func() {
			remoteConfigPath := "config/mci.yml"
			patch := &Patch{
				Patches: []ModulePatch{
					ModulePatch{
						PatchSet: PatchSet{
							Summary: []thirdparty.Summary{
								thirdparty.Summary{
									Name:      remoteConfigPath,
									Additions: 3,
									Deletions: 3,
								},
							},
						},
					},
				},
			}
			So(patch.ConfigChanged(remoteConfigPath), ShouldBeTrue)
		})

		Convey("the function should not return true if the config has not changed", func() {
			remoteConfigPath := "config/mci.yml"
			patch := &Patch{
				Patches: []ModulePatch{
					ModulePatch{
						PatchSet: PatchSet{
							Summary: []thirdparty.Summary{
								thirdparty.Summary{
									Name:      "dakar",
									Additions: 3,
									Deletions: 3,
								},
							},
						},
					},
				},
			}
			So(patch.ConfigChanged(remoteConfigPath), ShouldBeFalse)
		})
	})
}
