package model

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

// findMatrixVariant returns the variant representing a matrix value.
func findMatrixVariant(bvs []BuildVariant, cell matrixValue) *BuildVariant {
	for i, v := range bvs {
		found := 0
		for key, val := range cell {
			if x, ok := v.Expansions[key]; ok && x == val {
				found++
			}
		}
		if found == len(cell) {
			return &bvs[i]
		}
	}
	return nil
}

// findRegularVariant returns a non-matrix variant of the given id.
func findRegularVariant(bvs []BuildVariant, id string) *BuildVariant {
	for _, v := range bvs {
		if v.Name == id {
			return &v
		}
	}
	return nil
}

// taskNames returns list of task names give a variant definition.
func taskNames(v *BuildVariant) []string {
	var names []string
	for _, t := range v.Tasks {
		names = append(names, t.Name)
	}
	return names
}

func TestPythonMatrixIntegration(t *testing.T) {
	Convey("With a sample matrix project mocking up a python driver", t, func() {
		p := Project{}
		bytes, err := ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(),
			"testdata", "matrix_python.yml"))
		So(err, ShouldBeNil)
		Convey("the project should parse properly", func() {
			err := LoadProjectInto(bytes, "python", &p)
			So(err, ShouldBeNil)
			Convey("and contain the correct variants", func() {
				So(len(p.BuildVariants), ShouldEqual, (2*2*4 - 4))
				Convey("so that excluded matrix cells are not created", func() {
					So(findMatrixVariant(p.BuildVariants, matrixValue{
						"os": "windows", "python": "pypy", "c-extensions": "with-c",
					}), ShouldBeNil)
					So(findMatrixVariant(p.BuildVariants, matrixValue{
						"os": "windows", "python": "jython", "c-extensions": "with-c",
					}), ShouldBeNil)
					So(findMatrixVariant(p.BuildVariants, matrixValue{
						"os": "linux", "python": "pypy", "c-extensions": "with-c",
					}), ShouldBeNil)
					So(findMatrixVariant(p.BuildVariants, matrixValue{
						"os": "linux", "python": "jython", "c-extensions": "with-c",
					}), ShouldBeNil)
				})
				Convey("so that Windows builds without C extensions exclude LDAP tasks", func() {
					v := findMatrixVariant(p.BuildVariants, matrixValue{
						"os":           "windows",
						"python":       "python3",
						"c-extensions": "without-c",
					})
					So(v, ShouldNotBeNil)
					tasks := taskNames(v)
					So(len(tasks), ShouldEqual, 7)
					So(tasks, ShouldNotContain, "ldap_auth")
					So(v.DisplayName, ShouldEqual, "Windows 95 Python 3.0 (without C extensions)")
					So(v.RunOn, ShouldResemble, []string{"windows95-test"})
				})
				Convey("so that the linux/python3/c variant has a lint task", func() {
					v := findMatrixVariant(p.BuildVariants, matrixValue{
						"os":           "linux",
						"python":       "python3",
						"c-extensions": "with-c",
					})
					So(v, ShouldNotBeNil)
					tasks := taskNames(v)
					So(len(tasks), ShouldEqual, 9)
					So(tasks, ShouldContain, "ldap_auth")
					So(tasks, ShouldContain, "lint")
					So(v.DisplayName, ShouldEqual, "Linux Python 3.0 (with C extensions)")
					So(v.RunOn, ShouldResemble, []string{"centos6-perf"})
				})
			})
		})
	})
}

func TestDepsMatrixIntegration(t *testing.T) {
	Convey("With a sample matrix project mocking up a python driver", t, func() {
		p := Project{}
		bytes, err := ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(),
			"testdata", "matrix_deps.yml"))
		So(err, ShouldBeNil)
		Convey("the project should parse properly", func() {
			err := LoadProjectInto(bytes, "deps", &p)
			So(err, ShouldBeNil)
			Convey("and contain the correct variants", func() {
				So(len(p.BuildVariants), ShouldEqual, (1 + 3*3))
				Convey("including a non-matrix variant", func() {
					v := findRegularVariant(p.BuildVariants, "analysis")
					So(v, ShouldNotBeNil)
					ts := taskNames(v)
					So(ts, ShouldContain, "pre-task")
					So(ts, ShouldContain, "post-task")
					So(*v.Stepback, ShouldBeFalse)
				})
				Convey("including linux/standalone", func() {
					v := findMatrixVariant(p.BuildVariants, matrixValue{
						"os":            "linux",
						"configuration": "standalone",
					})
					So(v, ShouldNotBeNil)
					So(len(v.Tasks), ShouldEqual, 5)
					So(v.Tags, ShouldContain, "posix")
					Convey("which should contain a compile", func() {
						So(v.Tasks[4].Name, ShouldEqual, "compile")
						So(v.Tasks[4].Distros, ShouldResemble, []string{"linux_big"})
						So(v.Tasks[4].DependsOn[0], ShouldResemble, TaskUnitDependency{
							Name:    "pre-task",
							Variant: "analysis",
						})
					})
				})
				Convey("including osx/repl", func() {
					v := findMatrixVariant(p.BuildVariants, matrixValue{
						"os":            "osx",
						"configuration": "repl",
					})
					So(v, ShouldNotBeNil)
					So(len(v.Tasks), ShouldEqual, 4)
					So(v.Tags, ShouldContain, "posix")
					Convey("which should depend on another variant's compile", func() {
						So(v.Tasks[0].Name, ShouldEqual, "test1")
						So(v.Tasks[0].DependsOn[0].Name, ShouldEqual, "compile")
						So(v.Tasks[0].DependsOn[0].Variant, ShouldNotEqual, "")
					})
				})
			})

			Convey("and contain the correct tasks", func() {
				So(len(p.Tasks), ShouldEqual, 7)
				Convey("such that post-task depends on everything", func() {
					pt := ProjectTask{}
					for _, t := range p.Tasks {
						if t.Name == "post-task" {
							pt = t
						}
					}
					So(pt.Name, ShouldEqual, "post-task")
					So(len(pt.DependsOn), ShouldEqual, 4*(3*3))
				})
			})
		})
	})
}
