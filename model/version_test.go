package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/util"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(conf))
	mci.SetLogger("/tmp/version_test.log")
}

var versionTestSettings = mci.TestConfig()

func TestCreateStubVersionFromRevision(t *testing.T) {
	Convey("When calling CreateStubVersionFromRevision..", t, func() {
		util.HandleTestingErr(
			db.Clear(VersionsCollection), t,
			"error clearing versions collection")
		Convey("ensure all the version fields are set correctly", func() {
			projectRef := &ProjectRef{
				Identifier: "ident",
			}
			revision := &Revision{
				Revision: "revisionHash",
			}
			errs := []string{"a", "b"}
			expectedVersion := &Version{
				Id: CleanName(fmt.Sprintf("%v_%v", projectRef.String(),
					revision.Revision)),
				Identifier: projectRef.Identifier,
				Revision:   revision.Revision,
				Project:    projectRef.String(),
				Status:     mci.VersionCreated,
				Requester:  mci.RepotrackerVersionRequester,
				Errors:     errs,
			}
			_, err := CreateStubVersionFromRevision(projectRef, revision,
				versionTestSettings, errs)
			So(err, ShouldBeNil)
			// locate the created version and check pertinent fields
			version, err := FindVersion(expectedVersion.Id)
			So(version, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(version.Revision, ShouldResemble, expectedVersion.Revision)
			So(version.Errors, ShouldResemble, expectedVersion.Errors)
			So(version.Identifier, ShouldEqual, expectedVersion.Identifier)
			So(version.Status, ShouldResemble, expectedVersion.Status)
			So(version.Requester, ShouldResemble, expectedVersion.Requester)
			So(version.Config, ShouldResemble, "")
		})
	})
}

func TestCreateVersionFromRevision(t *testing.T) {
	Convey("When calling CreateVersionFromRevision..", t, func() {
		util.HandleTestingErr(
			db.Clear(VersionsCollection), t,
			"error clearing versions collection")
		Convey("ensure all the version fields are set correctly", func() {
			projectRef := &ProjectRef{
				Identifier: "ident",
			}
			revision := &Revision{
				Revision: "revisionHash",
			}
			errs := []string{"a", "b"}
			project := &Project{
				Repo: "repo",
			}
			expectedVersion := &Version{
				Id: CleanName(fmt.Sprintf("%v_%v", projectRef.String(),
					revision.Revision)),
				Identifier: projectRef.Identifier,
				Revision:   revision.Revision,
				Project:    projectRef.String(),
				Status:     mci.VersionCreated,
				Requester:  mci.RepotrackerVersionRequester,
				Errors:     errs,
			}
			_, err := CreateVersionFromRevision(projectRef, project,
				revision, versionTestSettings)

			So(err, ShouldBeNil)
			// locate the created version and check pertinent fields
			version, err := FindVersion(expectedVersion.Id)
			So(version, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(version.Revision, ShouldResemble, expectedVersion.Revision)
			So(len(version.Errors), ShouldEqual, 0)
			So(version.Status, ShouldResemble, expectedVersion.Status)
			So(version.Identifier, ShouldEqual, expectedVersion.Identifier)
			So(version.Requester, ShouldResemble, expectedVersion.Requester)
			So(version.Config, ShouldNotResemble, "")
		})
	})
}

func TestCreateProjectVersion(t *testing.T) {
	Convey("When calling CreateProjectVersion..", t, func() {
		util.HandleTestingErr(db.ClearCollections(VersionsCollection,
			RepositoriesCollection, BuildsCollection, TasksCollection), t,
			"Error clearing test collection")
		Convey("ensure all the version fields are set correctly", func() {
			projectRef := &ProjectRef{
				Identifier: "ident",
			}
			project := &Project{
				Repo: "repo",
				Tasks: []ProjectTask{
					ProjectTask{
						Name: "compile",
					},
				},
				BuildVariants: []BuildVariant{
					BuildVariant{
						Name: "bv",
						Tasks: []BuildVariantTask{
							BuildVariantTask{
								Name: "compile",
							},
						},
					},
				},
			}
			version := &Version{
				Id:         "version",
				Identifier: "ident",
			}
			_, err := createProjectVersion(projectRef, project, version,
				versionTestSettings)
			So(err, ShouldBeNil)
			// locate the created version and check pertinent fields
			version, err = FindVersion(version.Id)
			So(version, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(version.Remote, ShouldBeFalse)
			So(version.RemotePath, ShouldEqual, "")
			So(version.RevisionOrderNumber, ShouldEqual, 1)
			So(version.Identifier, ShouldEqual, "ident")
			So(version.RevisionOrderNumber, ShouldEqual, 1)
			So(len(version.BuildVariants), ShouldEqual, 1)
			So(version.BuildVariants[0].BuildVariant, ShouldEqual, "bv")
			So(version.BuildVariants[0].Activated, ShouldBeFalse)
			So(len(version.BuildIds), ShouldEqual, 1)
			// ensure the build was created
			build, err := FindBuild(version.BuildIds[0])
			So(build, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(len(build.Tasks), ShouldEqual, 1)
			// ensure the task was created
			task, err := FindTask(build.Tasks[0].Id)
			So(task, ShouldNotBeNil)
			So(err, ShouldBeNil)
		})
	})
}

func TestFindVersionByProjectAndRevision(t *testing.T) {
	Convey("When calling FindVersionByProjectAndRevision..", t, func() {
		projectRef := &ProjectRef{
			Identifier: "spongebob",
		}
		Convey("if there is a version corresponding to a project & revision"+
			" we should find it", func() {
			revision := &Revision{
				Revision: "squarepants",
			}

			insertedVersion := revisionToVersion(revision, projectRef)
			err := insertedVersion.Insert()
			So(err, ShouldBeNil)

			foundVersion, err := FindVersionByProjectAndRevision(projectRef, revision)
			So(err, ShouldBeNil)
			So(foundVersion, ShouldNotBeNil)
			So(foundVersion.Id, ShouldEqual, insertedVersion.Id)
			So(foundVersion.Revision, ShouldEqual, revision.Revision)
			So(foundVersion.Project, ShouldEqual, projectRef.String())
		})

		Convey("if there isn't a version corresponding to a project & revision"+
			" we should get back nil", func() {

			revision := &Revision{
				Revision: "squidward",
			}

			foundVersion, err := FindVersionByProjectAndRevision(projectRef, revision)
			So(err, ShouldBeNil)
			So(foundVersion, ShouldBeNil)
		})

		Reset(func() {
			db.Clear(VersionsCollection)
		})
	})
}

func TestVariantLastActivationTime(t *testing.T) {
	Convey("When calling FindVariantLastActivationTime...", t, func() {
		now := time.Now()
		firstVersionTime := now.Add(time.Duration(1) * time.Hour)
		secondVersionTime := now.Add(time.Duration(2) * time.Hour)

		proj := &ProjectRef{
			Identifier: "testproj",
		}

		bv1 := &BuildVariant{
			Name: "bv1",
		}

		bv2 := &BuildVariant{
			Name: "bv2",
		}

		v1 := Version{
			Id:       "bar",
			Revision: "bar",
			Project:  proj.String(),
			BuildVariants: []BuildStatus{
				BuildStatus{
					BuildVariant: bv1.Name,
					Activated:    true,
					ActivateAt:   firstVersionTime,
				},
				BuildStatus{
					BuildVariant: bv2.Name,
					Activated:    false,
					ActivateAt:   firstVersionTime,
				},
			},
			RevisionOrderNumber: 1,
			Requester:           mci.RepotrackerVersionRequester,
		}
		So(v1.Insert(), ShouldBeNil)
		v2 := Version{
			Id:       "foo",
			Revision: "foo",
			Project:  proj.String(),
			BuildVariants: []BuildStatus{
				BuildStatus{
					BuildVariant: bv1.Name,
					Activated:    true,
					ActivateAt:   secondVersionTime,
				},
				BuildStatus{
					BuildVariant: bv2.Name,
					Activated:    false,
					ActivateAt:   secondVersionTime,
				},
			},
			RevisionOrderNumber: 2,
			Requester:           mci.RepotrackerVersionRequester,
		}
		So(v2.Insert(), ShouldBeNil)
		Convey("the last variant activation time should be returned, if we have it", func() {
			la, err := variantLastActivationTime(proj, bv1)
			So(err, ShouldBeNil)
			So(la, ShouldNotBeNil)
			// compare parts since time returned from server has a loss of precision
			So(la.Day(), ShouldEqual, secondVersionTime.Day())
			So(la.Hour(), ShouldEqual, secondVersionTime.Hour())
			So(la.Minute(), ShouldEqual, secondVersionTime.Minute())

		})
		Convey("if we don't have a last activation time, we should return nil", func() {
			la, err := variantLastActivationTime(proj, bv2)
			So(err, ShouldBeNil)
			So(la, ShouldBeNil)
		})
		Reset(func() {
			db.Clear(VersionsCollection)
		})
	})
}

func TestLastKnownGoodConfig(t *testing.T) {
	Convey("When calling LastKnownGoodConfig..", t, func() {
		identifier := "identifier"
		Convey("no versions should be returned if there're no good "+
			"last known configurations", func() {
			version := &Version{
				Identifier: identifier,
				Requester:  mci.RepotrackerVersionRequester,
				Errors:     []string{"error 1", "error 2"},
			}
			util.HandleTestingErr(version.Insert(), t,
				"Error inserting test version: %v")
			versions, err := LastKnownGoodConfig(identifier)
			util.HandleTestingErr(err, t,
				"error calling LastKnownGoodConfig: %v")
			So(len(versions), ShouldEqual, 0)
		})
		Convey("a version should be returned if there is a last known good "+
			"configurations", func() {
			version := &Version{
				Identifier: identifier,
				Requester:  mci.RepotrackerVersionRequester,
			}
			util.HandleTestingErr(version.Insert(), t,
				"Error inserting test version: %v")
			versions, err := LastKnownGoodConfig(identifier)
			util.HandleTestingErr(err, t,
				"error calling LastKnownGoodConfig: %v")
			So(len(versions), ShouldEqual, 1)
		})
		Convey("the most version should be returned if there are several "+
			"last known good configurations", func() {
			version := &Version{
				Id:                  "1",
				Identifier:          identifier,
				Requester:           mci.RepotrackerVersionRequester,
				RevisionOrderNumber: 1,
				Config:              "1",
			}
			util.HandleTestingErr(version.Insert(), t,
				"Error inserting test version: %v")
			version.Id = "5"
			version.RevisionOrderNumber = 5
			version.Config = "5"
			util.HandleTestingErr(version.Insert(), t,
				"Error inserting test version: %v")
			version.Id = "2"
			version.RevisionOrderNumber = 2
			version.Config = "2"
			util.HandleTestingErr(version.Insert(), t,
				"Error inserting test version: %v")
			versions, err := LastKnownGoodConfig(identifier)
			util.HandleTestingErr(err, t,
				"error calling LastKnownGoodConfig: %v")
			So(len(versions), ShouldEqual, 3)
			So(versions[0].Config, ShouldEqual, "5")
		})
		Reset(func() {
			db.Clear(VersionsCollection)
		})
	})
}
