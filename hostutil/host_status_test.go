package hostutil

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	testConfig = evergreen.TestConfig()
)

func TestHostSshParse(t *testing.T) {
	Convey("Parsing a string with hostname and port should be parsed correctly", t, func() {
		hostname1 := "host:123"
		host1, err := util.ParseSSHInfo(hostname1)
		So(err, ShouldBeNil)
		So(host1.User, ShouldEqual, "")
		So(host1.Hostname, ShouldEqual, "host")
		So(host1.Port, ShouldEqual, "123")
	})

	Convey("Parsing a string with only a hostname should assume default port", t, func() {
		hostname2 := "host"
		host2, err := util.ParseSSHInfo(hostname2)
		So(err, ShouldBeNil)
		So(host2.User, ShouldEqual, "")
		So(host2.Hostname, ShouldEqual, "host")
		So(host2.Port, ShouldEqual, "22")
	})

	Convey("Parsing a string with user, hostname, and port should extract all 3", t, func() {
		hostname3 := "user@host:400"
		host3, err := util.ParseSSHInfo(hostname3)
		So(err, ShouldBeNil)
		So(host3.User, ShouldEqual, "user")
		So(host3.Hostname, ShouldEqual, "host")
		So(host3.Port, ShouldEqual, "400")
	})

	Convey("Parsing a string with user and host should assume the default port", t, func() {
		hostname4 := "user@host"
		host4, err := util.ParseSSHInfo(hostname4)
		So(err, ShouldBeNil)
		So(host4.User, ShouldEqual, "user")
		So(host4.Hostname, ShouldEqual, "host")
		So(host4.Port, ShouldEqual, "22")
	})
}
