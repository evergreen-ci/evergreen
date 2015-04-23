package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var (
	_ fmt.Stringer = nil
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(taskQueueTestConf))
}

func TestFindLastPassingVersionForBuildVariants(t *testing.T) {
	Convey("works", t, func() {
		So(db.Clear(TaskQueuesCollection), ShouldBeNil)

		project := "MyProject"
		bv1 := "linux"
		bv2 := "windows"
		projectObj := Project{
			Identifier: project,
		}

		insertVersion("1", 1, project)
		insertVersion("2", 2, project)
		insertVersion("3", 3, project)

		insertBuild("1a", project, bv1, mci.BuildSucceeded, 1)
		insertBuild("1b", project, bv2, mci.BuildSucceeded, 1)

		insertBuild("2a", project, bv1, mci.BuildSucceeded, 2)
		insertBuild("2b", project, bv2, mci.BuildSucceeded, 2)

		insertBuild("3a", project, bv1, mci.BuildSucceeded, 3)
		insertBuild("3b", project, bv2, mci.BuildFailed, 3)

		version, err := FindLastPassingVersionForBuildVariants(projectObj,
			[]string{bv1, bv2})

		So(err, ShouldBeNil)
		So(version, ShouldNotBeNil)
		So(version.Id, ShouldEqual, "2")
		So(version.RevisionOrderNumber, ShouldEqual, 2)
	})
}

func insertBuild(id string, project string, buildVariant string, status string,
	order int) {
	build := &Build{
		Id:                  id,
		Project:             project,
		BuildVariant:        buildVariant,
		Status:              status,
		Requester:           mci.RepotrackerVersionRequester,
		RevisionOrderNumber: order,
	}
	So(build.Insert(), ShouldBeNil)
}

func insertVersion(id string, order int, project string) {
	version := &Version{
		Id:                  id,
		RevisionOrderNumber: order,
		Project:             project,
		Requester:           mci.RepotrackerVersionRequester,
	}
	So(version.Insert(), ShouldBeNil)
}
