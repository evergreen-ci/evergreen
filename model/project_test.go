package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	projectTestConf = evergreen.TestConfig()
)

func TestFindProject(t *testing.T) {

	Convey("When finding a project", t, func() {

		Convey("an error should be thrown if the project ref is nil", func() {
			project, err := FindProject("", nil)
			So(err, ShouldNotBeNil)
			So(project, ShouldBeNil)
		})

		Convey("an error should be thrown if the project ref's identifier is nil", func() {
			projRef := &ProjectRef{
				Identifier: "",
			}
			project, err := FindProject("", projRef)
			So(err, ShouldNotBeNil)
			So(project, ShouldBeNil)
		})

		Convey("if the project file exists and is valid, the project spec within"+
			"should be unmarshalled and returned", func() {
			v := &version.Version{
				Owner:      "fakeowner",
				Repo:       "fakerepo",
				Branch:     "fakebranch",
				Identifier: "project_test",
				Requester:  evergreen.RepotrackerVersionRequester,
				Config:     "owner: fakeowner\nrepo: fakerepo\nbranch: fakebranch",
			}
			p := &ProjectRef{
				Identifier: "project_test",
				Owner:      "fakeowner",
				Repo:       "fakerepo",
				Branch:     "fakebranch",
			}
			testutil.HandleTestingErr(v.Insert(), t, "failed to insert test version: %v", v)
			_, err := FindProject("", p)
			So(err, ShouldBeNil)

		})

	})

}

func TestGetVariantMappings(t *testing.T) {

	Convey("With a project", t, func() {

		Convey("getting variant mappings should return a map of the build"+
			" variant names to their display names", func() {

			project := &Project{
				BuildVariants: []BuildVariant{
					{
						Name:        "bv1",
						DisplayName: "bv1",
					},
					{
						Name:        "bv2",
						DisplayName: "dsp2",
					},
					{
						Name:        "blecch",
						DisplayName: "blecchdisplay",
					},
				},
			}

			mappings := project.GetVariantMappings()
			So(len(mappings), ShouldEqual, 3)
			So(mappings["bv1"], ShouldEqual, "bv1")
			So(mappings["bv2"], ShouldEqual, "dsp2")
			So(mappings["blecch"], ShouldEqual, "blecchdisplay")

		})

	})

}

func TestGetVariantsWithTask(t *testing.T) {

	Convey("With a project", t, func() {

		project := &Project{
			BuildVariants: []BuildVariant{
				{
					Name:  "bv1",
					Tasks: []BuildVariantTask{{Name: "suite1"}},
				},
				{
					Name: "bv2",
					Tasks: []BuildVariantTask{
						{Name: "suite1"},
						{Name: "suite2"},
					},
				},
				{
					Name:  "bv3",
					Tasks: []BuildVariantTask{{Name: "suite2"}},
				},
			},
		}

		Convey("when getting the build variants where a task applies", func() {

			Convey("it should be run on any build variants where the test is"+
				" specified to run", func() {

				variants := project.GetVariantsWithTask("suite1")
				So(len(variants), ShouldEqual, 2)
				So(util.SliceContains(variants, "bv1"), ShouldBeTrue)
				So(util.SliceContains(variants, "bv2"), ShouldBeTrue)

				variants = project.GetVariantsWithTask("suite2")
				So(len(variants), ShouldEqual, 2)
				So(util.SliceContains(variants, "bv2"), ShouldBeTrue)
				So(util.SliceContains(variants, "bv3"), ShouldBeTrue)

			})

		})

	})
}

func TestGetModuleRepoName(t *testing.T) {

	Convey("With a module", t, func() {

		Convey("getting the repo owner and name should return the repo"+
			" field, split at the ':' and removing the .git from"+
			" the end", func() {

			module := &Module{
				Repo: "blecch:owner/repo.git",
			}

			owner, name := module.GetRepoOwnerAndName()
			So(owner, ShouldEqual, "owner")
			So(name, ShouldEqual, "repo")

		})

	})
}

func TestPopulateBVT(t *testing.T) {

	Convey("With a test Project and BuildVariantTask", t, func() {

		project := &Project{
			Tasks: []ProjectTask{
				{
					Name:            "task1",
					ExecTimeoutSecs: 500,
					Stepback:        new(bool),
					DependsOn:       []TaskDependency{{Name: "other"}},
					Priority:        1000,
					Patchable:       new(bool),
				},
			},
			BuildVariants: []BuildVariant{
				{
					Name:  "test",
					Tasks: []BuildVariantTask{{Name: "task1", Priority: 5}},
				},
			},
		}

		Convey("updating a BuildVariantTask with unset fields", func() {
			bvt := project.BuildVariants[0].Tasks[0]
			spec := project.GetSpecForTask("task1")
			So(spec.Name, ShouldEqual, "task1")
			bvt.Populate(spec)

			Convey("should inherit the unset fields from the Project", func() {
				So(bvt.Name, ShouldEqual, "task1")
				So(bvt.ExecTimeoutSecs, ShouldEqual, 500)
				So(bvt.Stepback, ShouldNotBeNil)
				So(bvt.Patchable, ShouldNotBeNil)
				So(len(bvt.DependsOn), ShouldEqual, 1)

				Convey("but not set fields", func() { So(bvt.Priority, ShouldEqual, 5) })
			})
		})

		Convey("updating a BuildVariantTask with set fields", func() {
			bvt := BuildVariantTask{
				Name:            "task1",
				ExecTimeoutSecs: 2,
				Stepback:        boolPtr(true),
				DependsOn:       []TaskDependency{{Name: "task2"}, {Name: "task3"}},
			}
			spec := project.GetSpecForTask("task1")
			So(spec.Name, ShouldEqual, "task1")
			bvt.Populate(spec)

			Convey("should not inherit set fields from the Project", func() {
				So(bvt.Name, ShouldEqual, "task1")
				So(bvt.ExecTimeoutSecs, ShouldEqual, 2)
				So(bvt.Stepback, ShouldNotBeNil)
				So(*bvt.Stepback, ShouldBeTrue)
				So(len(bvt.DependsOn), ShouldEqual, 2)

				Convey("but unset fields should", func() { So(bvt.Priority, ShouldEqual, 1000) })
			})
		})
	})
}

func TestIgnoresAllFiles(t *testing.T) {
	Convey("With test Project.Ignore setups and a list of.py, .yml, and .md files", t, func() {
		files := []string{
			"src/cool/test.py",
			"etc/other_config.yml",
			"README.md",
		}
		Convey("a project with an empty ignore field should never ignore files", func() {
			p := &Project{Ignore: []string{}}
			So(p.IgnoresAllFiles(files), ShouldBeFalse)
		})
		Convey("a project with a * ignore field should always ignore files", func() {
			p := &Project{Ignore: []string{"*"}}
			So(p.IgnoresAllFiles(files), ShouldBeTrue)
		})
		Convey("a project that ignores .py files should not ignore all files", func() {
			p := &Project{Ignore: []string{"*.py"}}
			So(p.IgnoresAllFiles(files), ShouldBeFalse)
		})
		Convey("a project that ignores .py, .yml, and .md files should ignore all files", func() {
			p := &Project{Ignore: []string{"*.py", "*.yml", "*.md"}}
			So(p.IgnoresAllFiles(files), ShouldBeTrue)
		})
		Convey("a project that ignores all files by name should ignore all files", func() {
			p := &Project{Ignore: []string{
				"src/cool/test.py",
				"etc/other_config.yml",
				"README.md",
			}}
			So(p.IgnoresAllFiles(files), ShouldBeTrue)
		})
		Convey("a project that ignores all files by dir should ignore all files", func() {
			p := &Project{Ignore: []string{"src/*", "etc/*", "README.md"}}
			So(p.IgnoresAllFiles(files), ShouldBeTrue)
		})
		Convey("a project with negations should not ignore all files", func() {
			p := &Project{Ignore: []string{"*", "!src/cool/*"}}
			So(p.IgnoresAllFiles(files), ShouldBeFalse)
		})
		Convey("a project with a negated filetype should not ignore all files", func() {
			p := &Project{Ignore: []string{"src/*", "!*.py", "*yml", "*.md"}}
			So(p.IgnoresAllFiles(files), ShouldBeFalse)
		})
	})
}

func boolPtr(b bool) *bool {
	return &b
}
