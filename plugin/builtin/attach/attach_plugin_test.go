package attach

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/user"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFileVisibility(t *testing.T) {

	Convey("With a list of files with 4 ui visibility permissions", t, func() {
		files := []artifact.File{
			{Name: "Private", Visibility: artifact.Private},
			{Name: "Public", Visibility: artifact.Public},
			{Name: "Hidden", Visibility: artifact.None},
			{Name: "Unset", Visibility: ""},
		}

		Convey("and no user", func() {
			stripped := stripHiddenFiles(files, nil)

			Convey("the original array should be unmodified", func() {
				So(len(files), ShouldEqual, 4)
			})

			Convey("and all only the public and unset files should be returned", func() {
				So(stripped[0].Name, ShouldEqual, "Public")
				So(stripped[1].Name, ShouldEqual, "Unset")
				So(len(stripped), ShouldEqual, 2)
			})
		})

		Convey("with a user", func() {
			stripped := stripHiddenFiles(files, &user.DBUser{})

			Convey("the original array should be unmodified", func() {
				So(len(files), ShouldEqual, 4)
			})

			Convey("and all but the 'None' files should be returned", func() {
				So(stripped[0].Name, ShouldEqual, "Private")
				So(stripped[1].Name, ShouldEqual, "Public")
				So(stripped[2].Name, ShouldEqual, "Unset")
				So(len(stripped), ShouldEqual, 3)
			})
		})
	})

}
