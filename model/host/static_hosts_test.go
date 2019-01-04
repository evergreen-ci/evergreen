package host

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecommissionInactiveStaticHosts(t *testing.T) {

	Convey("When decommissioning unused static hosts", t, func() {

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
			" '%v' collection", Collection)

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
				Provider: "ec2-spot",
			}
			So(inactiveEC2One.Insert(), ShouldBeNil)

			inactiveUnknownTypeOne := &Host{
				Id:     "inactiveUnknownTypeOne",
				Status: evergreen.HostRunning,
			}
			So(inactiveUnknownTypeOne.Insert(), ShouldBeNil)

			activeStaticHosts := []string{"activeStaticOne", "activeStaticTwo"}
			So(MarkInactiveStaticHosts(activeStaticHosts, ""), ShouldBeNil)

			found, err := Find(IsTerminated)
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 2)
			So(hostIdInSlice(found, inactiveOne.Id), ShouldBeTrue)
			So(hostIdInSlice(found, inactiveTwo.Id), ShouldBeTrue)
		})
	})
}

func TestTerminateStaticHostsForDistro(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))
	hosts := []Host{
		{
			Id:     "h1",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id: "d1",
			},
			Provider: evergreen.HostTypeStatic,
		},
		{
			Id:     "h2",
			Status: evergreen.HostQuarantined,
			Distro: distro.Distro{
				Id: "d1",
			},
			Provider: evergreen.HostTypeStatic,
		},
		{
			Id:     "h3",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id: "d1",
			},
			Provider: evergreen.HostTypeStatic,
		},
		{
			Id:     "h4",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id: "d2",
			},
			Provider: evergreen.HostTypeStatic,
		},
		{
			Id:     "5",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id: "d2",
			},
		},
	}
	for _, h := range hosts {
		assert.NoError(t, h.Insert())
	}
	found, err := Find(IsTerminated)
	assert.NoError(t, err)
	assert.Len(t, found, 0)
	assert.NoError(t, MarkInactiveStaticHosts([]string{}, "d1"))
	found, err = Find(IsTerminated)
	assert.NoError(t, err)
	assert.Len(t, found, 3)
	assert.NoError(t, MarkInactiveStaticHosts([]string{}, "d2"))
	found, err = Find(IsTerminated)
	assert.NoError(t, err)
	assert.Len(t, found, 4)
}
