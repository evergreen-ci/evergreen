package model

import (
	"10gen.com/mci"
	"10gen.com/mci/model/version"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var (
	projectTestConf = mci.TestConfig()
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
			" it should be unmarshalled and returned", func() {
			v := &version.Version{
				Owner:      "fakeowner",
				Repo:       "fakerepo",
				Branch:     "fakebranch",
				Identifier: "project_test",
				Requester:  mci.RepotrackerVersionRequester,
				Config:     "owner: fakeowner\nrepo: fakerepo\nbranch: fakebranch",
			}
			p := &ProjectRef{
				Identifier: "project_test",
				Owner:      "fakeowner",
				Repo:       "fakerepo",
				Branch:     "fakebranch",
			}
			util.HandleTestingErr(v.Insert(), t, "failed to insert test version: %v")
			project, err := FindProject("", p)
			So(err, ShouldBeNil)

			// check enough fields to make sure it was unmarshalled correctly
			So(project.Owner, ShouldEqual, "fakeowner")
			So(project.Repo, ShouldEqual, "fakerepo")
			So(project.Branch, ShouldEqual, "fakebranch")

		})

	})

}

func TestGetVariantMappings(t *testing.T) {

	Convey("With a project", t, func() {

		Convey("getting variant mappings should return a map of the build"+
			" variant names to their display names", func() {

			project := &Project{
				BuildVariants: []BuildVariant{
					BuildVariant{
						Name:        "bv1",
						DisplayName: "bv1",
					},
					BuildVariant{
						Name:        "bv2",
						DisplayName: "dsp2",
					},
					BuildVariant{
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
				BuildVariant{
					Name: "bv1",
					Tasks: []BuildVariantTask{
						BuildVariantTask{
							Name: "suite1",
						},
					},
				},
				BuildVariant{
					Name: "bv2",
					Tasks: []BuildVariantTask{
						BuildVariantTask{
							Name: "suite1",
						},
						BuildVariantTask{
							Name: "suite2",
						},
					},
				},
				BuildVariant{
					Name: "bv3",
					Tasks: []BuildVariantTask{
						BuildVariantTask{
							Name: "suite2",
						},
					},
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

func TestRepoUrl(t *testing.T) {

	Convey("With a project", t, func() {

		project := &Project{
			Owner: "owner",
			Repo:  "repo",
		}

		Convey("when getting the repo url", func() {

			Convey("with a git repo, a correct git url should be built based"+
				" on the owner and repo", func() {
				project.RepoKind = GithubRepoType
				location, err := project.Location()
				So(location, ShouldEqual, "git@github.com:owner/repo.git")
				So(err, ShouldBeNil)
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

func TestBuildVariantMatrix(t *testing.T) {
	Convey("Should be able to build up build variants from a matrix", t, func() {
		expansions := make(map[string]string)
		templateBv := BuildVariant{
			Expansions: expansions,
			Name:       "${mongodb_version}/${python_version}",
			RunOn:      []string{"ubuntu_${python_version}"},
		}

		templateBv.Expansions["python"] = "${python_version}"

		parameters := []MatrixParameter{
			MatrixParameter{
				Name: "mongodb_version",
				Values: []MatrixParameterValue{
					MatrixParameterValue{
						Value: "2.2.7",
						Expansions: map[string]string{
							"special_flags": "--version=${python_version}",
						},
					},
					MatrixParameterValue{Value: "2.4.10"},
					MatrixParameterValue{Value: "2.6.1"},
				},
			},
			MatrixParameter{
				Name: "python_version",
				Values: []MatrixParameterValue{
					MatrixParameterValue{Value: "3.0"},
					MatrixParameterValue{Value: "3.1"},
				},
			},
		}

		bvMatrix := BuildVariantMatrix{
			Template:         templateBv,
			MatrixParameters: parameters,
		}

		project := &Project{
			Owner:              "owner",
			Repo:               "repo",
			BuildVariantMatrix: bvMatrix,
		}

		err := addMatrixVariants(project)
		So(err, ShouldBeNil)

		So(len(project.BuildVariants), ShouldEqual, 6)

		So(project.BuildVariants[0].Expansions["python"], ShouldEqual, "3.0")
		So(project.BuildVariants[0].Expansions["special_flags"], ShouldEqual,
			"--version=3.0")
		So(project.BuildVariants[0].Name, ShouldEqual, "2.2.7/3.0")
		So(len(project.BuildVariants[0].RunOn), ShouldEqual, 1)
		So(project.BuildVariants[0].RunOn[0], ShouldEqual, "ubuntu_3.0")
		So(project.BuildVariants[0].MatrixParameterValues["mongodb_version"],
			ShouldEqual, "2.2.7")
		So(project.BuildVariants[0].MatrixParameterValues["python_version"],
			ShouldEqual, "3.0")

		So(project.BuildVariants[1].Expansions["python"], ShouldEqual, "3.1")
		So(project.BuildVariants[1].Expansions["special_flags"], ShouldEqual,
			"--version=3.1")
		So(project.BuildVariants[1].Name, ShouldEqual, "2.2.7/3.1")
		So(len(project.BuildVariants[1].RunOn), ShouldEqual, 1)
		So(project.BuildVariants[1].RunOn[0], ShouldEqual, "ubuntu_3.1")
		So(project.BuildVariants[1].MatrixParameterValues["mongodb_version"],
			ShouldEqual, "2.2.7")
		So(project.BuildVariants[1].MatrixParameterValues["python_version"],
			ShouldEqual, "3.1")

		So(project.BuildVariants[2].Expansions["python"], ShouldEqual, "3.0")
		So(project.BuildVariants[2].Expansions["special_flags"], ShouldEqual, "")
		So(project.BuildVariants[2].Name, ShouldEqual, "2.4.10/3.0")
		So(len(project.BuildVariants[2].RunOn), ShouldEqual, 1)
		So(project.BuildVariants[2].RunOn[0], ShouldEqual, "ubuntu_3.0")
		So(project.BuildVariants[2].MatrixParameterValues["mongodb_version"],
			ShouldEqual, "2.4.10")
		So(project.BuildVariants[2].MatrixParameterValues["python_version"],
			ShouldEqual, "3.0")

		So(project.BuildVariants[3].Expansions["python"], ShouldEqual, "3.1")
		So(project.BuildVariants[3].Expansions["special_flags"], ShouldEqual, "")
		So(project.BuildVariants[3].Name, ShouldEqual, "2.4.10/3.1")
		So(len(project.BuildVariants[3].RunOn), ShouldEqual, 1)
		So(project.BuildVariants[3].RunOn[0], ShouldEqual, "ubuntu_3.1")
		So(project.BuildVariants[3].MatrixParameterValues["mongodb_version"],
			ShouldEqual, "2.4.10")
		So(project.BuildVariants[3].MatrixParameterValues["python_version"],
			ShouldEqual, "3.1")

		So(project.BuildVariants[4].Expansions["python"], ShouldEqual, "3.0")
		So(project.BuildVariants[4].Expansions["special_flags"], ShouldEqual, "")
		So(project.BuildVariants[4].Name, ShouldEqual, "2.6.1/3.0")
		So(len(project.BuildVariants[4].RunOn), ShouldEqual, 1)
		So(project.BuildVariants[4].RunOn[0], ShouldEqual, "ubuntu_3.0")
		So(project.BuildVariants[4].MatrixParameterValues["mongodb_version"],
			ShouldEqual, "2.6.1")
		So(project.BuildVariants[4].MatrixParameterValues["python_version"],
			ShouldEqual, "3.0")

		So(project.BuildVariants[5].Expansions["python"], ShouldEqual, "3.1")
		So(project.BuildVariants[5].Expansions["special_flags"], ShouldEqual, "")
		So(project.BuildVariants[5].Name, ShouldEqual, "2.6.1/3.1")
		So(len(project.BuildVariants[5].RunOn), ShouldEqual, 1)
		So(project.BuildVariants[5].RunOn[0], ShouldEqual, "ubuntu_3.1")
		So(project.BuildVariants[5].MatrixParameterValues["mongodb_version"],
			ShouldEqual, "2.6.1")
		So(project.BuildVariants[5].MatrixParameterValues["python_version"],
			ShouldEqual, "3.1")
	})

	Convey("should do nothing if there are no parameters", t, func() {
		project := &Project{
			Owner: "owner",
			Repo:  "repo",

			BuildVariants: []BuildVariant{
				BuildVariant{
					Name: "test",
				},
			},
		}

		addMatrixVariants(project)
		So(len(project.BuildVariants), ShouldEqual, 1)
	})
}
