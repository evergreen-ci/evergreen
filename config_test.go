package evergreen

import (
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

//Checks that the test settings file can be parsed
//and returns a settings object.
func TestInitSettings(t *testing.T) {
	Convey("Parsing a valid settings file should succeed", t, func() {
		_, err := NewSettings(filepath.Join(FindEvergreenHome(),
			"testdata", "mci_settings.yml"))
		So(err, ShouldBeNil)
	})
}

//Checks that trying to parse a non existent file returns non-nil err
func TestBadInit(t *testing.T) {
	Convey("Parsing a nonexistent config file should cause an error", t, func() {
		_, err := NewSettings(filepath.Join(FindEvergreenHome(),
			"testdata", "blahblah.yml"))
		So(err, ShouldNotBeNil)
	})
}
