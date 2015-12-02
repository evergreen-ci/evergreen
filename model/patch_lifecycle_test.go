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

func TestIncludePatchDependencies(t *testing.T) {
	Convey("With a project task config with cross-variant dependencies", t, func() {
		p := &Project{
			Tasks: []ProjectTask{
				{Name: "t1"},
				{Name: "t2", DependsOn: []TaskDependency{{Name: "t1"}}},
				{Name: "t3"},
				{Name: "t4", Patchable: new(bool)},
				{Name: "t5", DependsOn: []TaskDependency{{Name: "t4"}}},
			},
			BuildVariants: []BuildVariant{
				{Name: "v1", Tasks: []BuildVariantTask{{Name: "t1"}, {Name: "t2"}}},
				{Name: "v2", Tasks: []BuildVariantTask{
					{Name: "t3", DependsOn: []TaskDependency{{Name: "t2", Variant: "v1"}}}}},
			},
		}

		Convey("a patch against v1/t1 should remain unchanged", func() {
			variants, tasks := IncludePatchDependencies(p, []string{"v1"}, []string{"t1"})
			So(len(variants), ShouldEqual, 1)
			So(variants, ShouldContain, "v1")
			So(len(tasks), ShouldEqual, 1)
			So(tasks, ShouldContain, "t1")
		})

		Convey("a patch against v1/t2 should add t1", func() {
			variants, tasks := IncludePatchDependencies(p, []string{"v1"}, []string{"t2"})
			So(len(variants), ShouldEqual, 1)
			So(variants, ShouldContain, "v1")
			So(len(tasks), ShouldEqual, 2)
			So(tasks, ShouldContain, "t1")
			So(tasks, ShouldContain, "t2")
		})

		Convey("a patch against v2/t3 should add t1,t2, and v1", func() {
			variants, tasks := IncludePatchDependencies(p, []string{"v2"}, []string{"t3"})
			So(len(variants), ShouldEqual, 2)
			So(variants, ShouldContain, "v1")
			So(variants, ShouldContain, "v2")
			So(len(tasks), ShouldEqual, 3)
			So(tasks, ShouldContain, "t1")
			So(tasks, ShouldContain, "t2")
			So(tasks, ShouldContain, "t3")
		})

		Convey("a patch against v2/t5 should be pruned, since its dependeny is not patchable", func() {
			variants, tasks := IncludePatchDependencies(p, []string{"v2"}, []string{"t5"})
			So(len(variants), ShouldEqual, 1)
			So(variants, ShouldContain, "v2")
			So(len(tasks), ShouldEqual, 0)
		})
	})

	Convey("With a project task config with * selectors", t, func() {
		p := &Project{
			Tasks: []ProjectTask{
				{Name: "t1"},
				{Name: "t2"},
				{Name: "t3", DependsOn: []TaskDependency{{Name: AllDependencies}}},
				{Name: "t4", DependsOn: []TaskDependency{{Name: "t3", Variant: AllVariants}}},
				{Name: "t5", DependsOn: []TaskDependency{{Name: AllDependencies, Variant: AllVariants}}},
			},
			BuildVariants: []BuildVariant{
				{Name: "v1", Tasks: []BuildVariantTask{{Name: "t1"}, {Name: "t2"}, {Name: "t3"}}},
				{Name: "v2", Tasks: []BuildVariantTask{{Name: "t1"}, {Name: "t2"}, {Name: "t3"}}},
				{Name: "v3", Tasks: []BuildVariantTask{{Name: "t4"}}},
				{Name: "v4", Tasks: []BuildVariantTask{{Name: "t5"}}},
			},
		}

		Convey("a patch against v1/t3 should include t2 and t1", func() {
			variants, tasks := IncludePatchDependencies(p, []string{"v1"}, []string{"t3"})
			So(len(variants), ShouldEqual, 1)
			So(variants, ShouldContain, "v1")
			So(len(tasks), ShouldEqual, 3)
			So(tasks, ShouldContain, "t1")
			So(tasks, ShouldContain, "t2")
			So(tasks, ShouldContain, "t3")
		})

		Convey("a patch against v3/t4 should include v1, v2, t3, t2, and t1", func() {
			variants, tasks := IncludePatchDependencies(p, []string{"v3"}, []string{"t4"})
			So(len(variants), ShouldEqual, 3)
			So(variants, ShouldContain, "v1")
			So(variants, ShouldContain, "v2")
			So(variants, ShouldContain, "v3")
			So(len(tasks), ShouldEqual, 4)
			So(tasks, ShouldContain, "t1")
			So(tasks, ShouldContain, "t2")
			So(tasks, ShouldContain, "t3")
			So(tasks, ShouldContain, "t4")
		})

		Convey("a patch against v4/t5 should include v1, v2, v3, t4, t3, t2, and t1", func() {
			variants, tasks := IncludePatchDependencies(p, []string{"v4"}, []string{"t5"})
			So(len(variants), ShouldEqual, 4)
			So(variants, ShouldContain, "v1")
			So(variants, ShouldContain, "v2")
			So(variants, ShouldContain, "v3")
			So(variants, ShouldContain, "v4")
			So(len(tasks), ShouldEqual, 5)
			So(tasks, ShouldContain, "t1")
			So(tasks, ShouldContain, "t2")
			So(tasks, ShouldContain, "t3")
			So(tasks, ShouldContain, "t4")
			So(tasks, ShouldContain, "t5")
		})
	})

}
