package remote

import (
	"testing"

	"github.com/evergreen-ci/evergreen/command"
	. "github.com/smartystreets/goconvey/convey"
)

func TestSFTPGateway(t *testing.T) {
	t.Skipf("the fixtures for remote ssh are not properly configured/conceived.")

	Convey("When using an SFTPGateway", t, func() {
		gateway := &SFTPGateway{
			Host:    command.TestRemote + ":22",
			User:    command.TestRemoteUser,
			Keyfile: command.TestRemoteKey,
		}

		Convey("the encapsualted client should be usable after the Init() call", func() {

			So(gateway.Init(), ShouldBeNil)
			defer gateway.Close()

			So(gateway.Client, ShouldNotBeNil)
			file, err := gateway.Client.Create("test.txt")
			So(err, ShouldBeNil)
			So(file, ShouldNotBeNil)
			So(file.Close(), ShouldBeNil)

		})

	})

}
