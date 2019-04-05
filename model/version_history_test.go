package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFindLastPassingVersionForBuildVariants(t *testing.T) {
	Convey("works", t, func() {
		So(db.ClearCollections(TaskQueuesCollection, VersionCollection, build.Collection), ShouldBeNil)

		project := "MyProject"
		bv1 := "linux"
		bv2 := "windows"
		projectObj := Project{
			Identifier: project,
		}

		insertVersion("1", 1, project)
		insertVersion("2", 2, project)
		insertVersion("3", 3, project)

		insertBuild("1a", project, bv1, evergreen.BuildSucceeded, 1)
		insertBuild("1b", project, bv2, evergreen.BuildSucceeded, 1)
		insertPatchBuild("1ap", project, bv1, evergreen.BuildSucceeded, 1)
		insertPatchBuild("1bp", project, bv2, evergreen.BuildSucceeded, 1)

		insertBuild("2a", project, bv1, evergreen.BuildSucceeded, 2)
		insertBuild("2b", project, bv2, evergreen.BuildSucceeded, 2)
		insertPatchBuild("2ap", project, bv1, evergreen.BuildSucceeded, 2)
		insertPatchBuild("2bp", project, bv2, evergreen.BuildSucceeded, 2)

		insertBuild("3a", project, bv1, evergreen.BuildSucceeded, 3)
		insertBuild("3b", project, bv2, evergreen.BuildFailed, 3)
		insertPatchBuild("3ap", project, bv1, evergreen.BuildSucceeded, 3)
		insertPatchBuild("3bp", project, bv2, evergreen.BuildFailed, 3)

		version, err := FindLastPassingVersionForBuildVariants(&projectObj, []string{bv1, bv2})

		So(err, ShouldBeNil)
		So(version, ShouldNotBeNil)
		So(version.Id, ShouldEqual, "2")
		So(version.RevisionOrderNumber, ShouldEqual, 2)
	})
}

func insertBuild(id string, project string, buildVariant string, status string, order int) {
	b := &build.Build{
		Id:                  id,
		Project:             project,
		BuildVariant:        buildVariant,
		Status:              status,
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: order,
	}
	So(b.Insert(), ShouldBeNil)
}

func insertPatchBuild(id string, project string, buildVariant string, status string, order int) {
	b := &build.Build{
		Id:                  id,
		Project:             project,
		BuildVariant:        buildVariant,
		Status:              status,
		Requester:           evergreen.GithubPRRequester,
		RevisionOrderNumber: order,
	}
	So(b.Insert(), ShouldBeNil)
}

func insertVersion(id string, order int, project string) {
	v := &Version{
		Id:                  id,
		RevisionOrderNumber: order,
		Identifier:          project,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	So(v.Insert(), ShouldBeNil)
}
