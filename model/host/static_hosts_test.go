package host

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestDecommissionInactiveStaticHosts(t *testing.T) {

	Convey("When decommissioning unused static hosts", t, func() {

		util.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
			" '%v' collection", Collection)

		Convey("if a nil slice is passed in, no host(s) should"+
			" be decommissioned in the database", func() {

			inactiveOne := &Host{
				Id:       "inactiveOne",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeStatic,
			}
			So(inactiveOne.Insert(), ShouldBeNil)

			inactiveTwo := &Host{
				Id:       "inactiveTwo",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeStatic,
			}
			So(inactiveTwo.Insert(), ShouldBeNil)

			So(DecommissionInactiveStaticHosts(nil), ShouldBeNil)

			found, err := Find(All)
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 2)
			So(found[0].Status, ShouldEqual, mci.HostRunning)
			So(found[1].Status, ShouldEqual, mci.HostRunning)

		})

		Convey("if a non-nil slice is passed in, any static hosts with ids not in"+
			" the slice should be removed from the database", func() {

			activeOne := &Host{
				Id:       "activeStaticOne",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeStatic,
			}
			So(activeOne.Insert(), ShouldBeNil)

			activeTwo := &Host{
				Id:       "activeStaticTwo",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeStatic,
			}
			So(activeTwo.Insert(), ShouldBeNil)

			inactiveOne := &Host{
				Id:       "inactiveStaticOne",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeStatic,
			}
			So(inactiveOne.Insert(), ShouldBeNil)

			inactiveTwo := &Host{
				Id:       "inactiveStaticTwo",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeStatic,
			}
			So(inactiveTwo.Insert(), ShouldBeNil)

			inactiveEC2One := &Host{
				Id:       "inactiveEC2One",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeEC2,
			}
			So(inactiveEC2One.Insert(), ShouldBeNil)

			inactiveUnknownTypeOne := &Host{
				Id:     "inactiveUnknownTypeOne",
				Status: mci.HostRunning,
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
