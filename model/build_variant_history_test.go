package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func dropTestDB(t *testing.T) {
	session, _, err := db.GetGlobalSessionFactory().GetSession()
	testutil.HandleTestingErr(err, t, "Error opening database session")
	defer session.Close()
	testutil.HandleTestingErr(session.DB(testConfig.Database.DB).DropDatabase(), t, "Error dropping test database")
}

func createVersion(order int, project string, buildVariants []string) error {
	v := &version.Version{}
	testActivationTime := time.Now().Add(time.Duration(4) * time.Hour)

	for _, variant := range buildVariants {
		v.BuildVariants = append(v.BuildVariants, version.BuildStatus{
			BuildVariant: variant,
			Activated:    false,
			ActivateAt:   testActivationTime,
		})
	}
	v.RevisionOrderNumber = order
	v.Identifier = project
	v.Id = fmt.Sprintf("version_%v_%v", order, project)
	v.Requester = evergreen.RepotrackerVersionRequester
	return v.Insert()
}

func createTask(id string, order int, project string, buildVariant string, gitspec string) error {
	task := &task.Task{}
	task.BuildVariant = buildVariant
	task.RevisionOrderNumber = order
	task.Project = project
	task.Revision = gitspec
	task.DisplayName = id
	task.Id = id
	task.Requester = evergreen.RepotrackerVersionRequester
	return task.Insert()
}

func TestBuildVariantHistoryIterator(t *testing.T) {
	dropTestDB(t)

	Convey("Should return the correct tasks and versions", t, func() {
		So(createVersion(1, "project1", []string{"bv1", "bv2"}), ShouldBeNil)
		So(createVersion(1, "project2", []string{"bv1", "bv2"}), ShouldBeNil)
		So(createVersion(2, "project1", []string{"bv1", "bv2"}), ShouldBeNil)
		So(createVersion(3, "project1", []string{"bv2"}), ShouldBeNil)

		So(createTask("task1", 1, "project1", "bv1", "gitspec0"), ShouldBeNil)
		So(createTask("task2", 1, "project1", "bv2", "gitspec0"), ShouldBeNil)
		So(createTask("task3", 1, "project2", "bv1", "gitspec1"), ShouldBeNil)
		So(createTask("task4", 1, "project2", "bv2", "gitspec1"), ShouldBeNil)
		So(createTask("task5", 2, "project1", "bv1", "gitspec2"), ShouldBeNil)
		So(createTask("task6", 2, "project1", "bv2", "gitspec2"), ShouldBeNil)
		So(createTask("task7", 3, "project1", "bv2", "gitspec3"), ShouldBeNil)

		Convey("Should respect project and build variant rules", func() {
			iter := NewBuildVariantHistoryIterator("bv1", "bv1", "project1")

			tasks, versions, err := iter.GetItems(nil, 5)
			So(err, ShouldBeNil)
			So(len(versions), ShouldEqual, 2)
			// Versions on project1 that have `bv1` in their build variants list
			So(versions[0].Id, ShouldEqual, "version_2_project1")
			So(versions[1].Id, ShouldEqual, "version_1_project1")

			// Tasks with order >= 1 s.t. project == `project1` and build_variant == `bv1`
			So(len(tasks), ShouldEqual, 2)
			So(tasks[0]["_id"], ShouldEqual, "gitspec2")
			So(tasks[1]["_id"], ShouldEqual, "gitspec0")
		})
	})
}
