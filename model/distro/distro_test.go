package distro

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mci.TestConfig()))
}

const distrosDir = "testdata"

func TestLoadDistrosFromDirectory(t *testing.T) {

	Convey("When loading in distros from a specified directory", t, func() {

		Convey("all non-template distros in .yml files should be picked"+
			" up, ignoring template distros and distros in non-yml"+
			" files", func() {

			distros, err := LoadFromDirectory(distrosDir)
			So(err, ShouldBeNil)
			So(len(distros), ShouldEqual, 3)
			So(distros["d1"].Name, ShouldEqual, "d1")
			So(distros["d2"].Name, ShouldEqual, "d2")
			So(distros["d3"].Name, ShouldEqual, "d3")

		})

	})

}

func TestLoadOneDistroFromDirectory(t *testing.T) {

	Convey("When loading in a single distro from a specified"+
		" directory", t, func() {

		Convey("if the distro does not exist, an error should be"+
			" returned", func() {

			distro, err := LoadOneFromDirectory(distrosDir, "dBad")
			So(err, ShouldNotBeNil)
			So(distro, ShouldBeNil)

		})

		Convey("if the distro exists in the directory, it should be"+
			" returned", func() {

			distro, err := LoadOneFromDirectory(distrosDir, "d1")
			So(err, ShouldBeNil)
			So(distro, ShouldNotBeNil)
			So(distro.Name, ShouldEqual, "d1")

		})

	})

}
