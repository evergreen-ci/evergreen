package host

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecommissionInactiveStaticHosts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When decommissioning unused static hosts", t, func() {

		require.NoError(t, db.Clear(Collection))

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
				Provider: "ec2-ondemand",
			}
			So(inactiveEC2One.Insert(), ShouldBeNil)

			inactiveUnknownTypeOne := &Host{
				Id:     "inactiveUnknownTypeOne",
				Status: evergreen.HostRunning,
			}
			So(inactiveUnknownTypeOne.Insert(), ShouldBeNil)

			activeStaticHosts := []string{"activeStaticOne", "activeStaticTwo"}
			So(MarkInactiveStaticHosts(ctx, activeStaticHosts, nil), ShouldBeNil)

			found, err := Find(IsTerminated)
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 2)
			So(hostIdInSlice(found, inactiveOne.Id), ShouldBeTrue)
			So(hostIdInSlice(found, inactiveTwo.Id), ShouldBeTrue)
		})
	})
}

func TestTerminateStaticHostsForDistro(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	d := &distro.Distro{Id: "d1"}
	assert.NoError(t, MarkInactiveStaticHosts(ctx, []string{}, d))
	found, err = Find(IsTerminated)
	assert.NoError(t, err)
	assert.Len(t, found, 3)
	d2 := &distro.Distro{Id: "d2"}
	assert.NoError(t, MarkInactiveStaticHosts(ctx, []string{}, d2))
	found, err = Find(IsTerminated)
	assert.NoError(t, err)
	assert.Len(t, found, 4)
}

func TestTerminateStaticHostsForDistroAliases(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id: "d1",
			},
			Provider: evergreen.HostTypeStatic,
		},
		{
			Id:     "h3",
			Status: evergreen.HostQuarantined,
			Distro: distro.Distro{
				Id: "dAlias",
			},
			Provider: evergreen.HostTypeStatic,
		},
		{
			Id:     "h4",
			Status: evergreen.HostRunning,
			Distro: distro.Distro{
				Id: "dAlias",
			},
			Provider: evergreen.HostTypeStatic,
		},
	}

	for _, h := range hosts {
		assert.NoError(t, h.Insert())
	}
	d := &distro.Distro{
		Id:      "d1",
		Aliases: []string{"dAlias"},
	}
	assert.NoError(t, MarkInactiveStaticHosts(ctx, []string{"h1"}, d))
	found, err := Find(IsTerminated)
	assert.NoError(t, err)
	assert.Len(t, found, 3)
}
