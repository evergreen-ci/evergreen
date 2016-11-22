package host

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers/ec2"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDecommissionInactiveStaticHosts(t *testing.T) {

	Convey("When decommissioning unused static hosts", t, func() {

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
			" '%v' collection", Collection)

		Convey("if a nil slice is passed in, no host(s) should"+
			" be decommissioned in the database", func() {

			inactiveOne := &Host{
				Id:       "inactiveOne",
				Status:   evergreen.HostRunning,
				Provider: evergreen.HostTypeStatic,
			}
			So(inactiveOne.Insert(), ShouldBeNil)

			inactiveTwo := &Host{
				Id:       "inactiveTwo",
				Status:   evergreen.HostRunning,
				Provider: evergreen.HostTypeStatic,
			}
			So(inactiveTwo.Insert(), ShouldBeNil)

			So(DecommissionInactiveStaticHosts(nil), ShouldBeNil)

			found, err := Find(All)
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 2)
			So(found[0].Status, ShouldEqual, evergreen.HostRunning)
			So(found[1].Status, ShouldEqual, evergreen.HostRunning)

		})

		Convey("if a non-nil slice is passed in, any static hosts with ids not in"+
			" the slice should be removed from the database", func() {

			activeOne := &Host{
				Id:       "activeStaticOne",
				Status:   evergreen.HostRunning,
				Provider: evergreen.HostTypeStatic,
			}
			So(activeOne.Insert(), ShouldBeNil)

			activeTwo := &Host{
				Id:       "activeStaticTwo",
				Status:   evergreen.HostRunning,
				Provider: evergreen.HostTypeStatic,
			}
			So(activeTwo.Insert(), ShouldBeNil)

			inactiveOne := &Host{
				Id:       "inactiveStaticOne",
				Status:   evergreen.HostRunning,
				Provider: evergreen.HostTypeStatic,
			}
			So(inactiveOne.Insert(), ShouldBeNil)

			inactiveTwo := &Host{
				Id:       "inactiveStaticTwo",
				Status:   evergreen.HostRunning,
				Provider: evergreen.HostTypeStatic,
			}
			So(inactiveTwo.Insert(), ShouldBeNil)

			inactiveEC2One := &Host{
				Id:       "inactiveEC2One",
				Status:   evergreen.HostRunning,
				Provider: ec2.SpotProviderName,
			}
			So(inactiveEC2One.Insert(), ShouldBeNil)

			inactiveUnknownTypeOne := &Host{
				Id:     "inactiveUnknownTypeOne",
				Status: evergreen.HostRunning,
			}
			So(inactiveUnknownTypeOne.Insert(), ShouldBeNil)

			activeStaticHosts := []string{"activeStaticOne", "activeStaticTwo"}
			So(DecommissionInactiveStaticHosts(activeStaticHosts), ShouldBeNil)

			found, err := Find(IsDecommissioned)
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 2)
			So(hostIdInSlice(found, inactiveOne.Id), ShouldBeTrue)
			So(hostIdInSlice(found, inactiveTwo.Id), ShouldBeTrue)
		})
	})
}
