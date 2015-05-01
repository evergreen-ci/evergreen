package remote

import (
	"github.com/evergreen-ci/evergreen/command"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestSFTPGateway(t *testing.T) {

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
