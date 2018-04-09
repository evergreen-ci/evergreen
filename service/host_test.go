package service

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	. "github.com/smartystreets/goconvey/convey"
)

func TestModifyHostStatus(t *testing.T) {
	Convey("Changing host status through the UI", t, func() {
		Convey("should allow users to change hosts from 'running' to 'undefined'", func() {
			user1 := user.DBUser{Id: "user1"}
			h1 := host.Host{Id: "h1", Status: evergreen.HostRunning}
			opts1 := uiParams{Action: "updateStatus", Status: evergreen.HostQuarantined}

			result, err := modifyHostStatus(&h1, &opts1, &user1)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, fmt.Sprintf(HostStatusUpdateSuccess, evergreen.HostRunning, evergreen.HostQuarantined))
			So(h1.Status, ShouldEqual, evergreen.HostQuarantined)
		})

		Convey("should not allow users to decommission static hosts", func() {
			user2 := user.DBUser{Id: "user2"}
			h2 := host.Host{Id: "h2", Status: evergreen.HostRunning, Provider: evergreen.ProviderNameStatic}
			opts2 := uiParams{Action: "updateStatus", Status: evergreen.HostDecommissioned}

			_, err := modifyHostStatus(&h2, &opts2, &user2)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, DecommissionStaticHostError)
		})
	})
}
