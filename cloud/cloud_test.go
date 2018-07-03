package cloud

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestGetManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Convey("GetManager() should return non-nil for all valid provider names", t, func() {

		Convey("EC2Auto should be returned for ec2-auto provider name", func() {
			cloudMgr, err := GetManager(ctx, "ec2-auto", testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &ec2Manager{})
		})

		Convey("EC2Spot should be returned for ec2-spot provider name", func() {
			cloudMgr, err := GetManager(ctx, "ec2-spot", testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &ec2Manager{})
		})

		Convey("EC2 should be returned for ec2 provider name", func() {
			cloudMgr, err := GetManager(ctx, "ec2-ondemand", testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &ec2Manager{})
		})

		Convey("Static should be returned for static provider name", func() {
			cloudMgr, err := GetManager(ctx, "static", testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &staticManager{})
		})

		Convey("Mock should be returned for mock provider name", func() {
			cloudMgr, err := GetManager(ctx, "mock", testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &mockManager{})
		})

		Convey("Invalid provider names should return nil with err", func() {
			cloudMgr, err := GetManager(ctx, "bogus", testutil.TestConfig())
			So(cloudMgr, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})

}

func TestGetContainerManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)

	cloudMgr, err := GetContainerManager(ctx, "docker", testutil.TestConfig())
	assert.NotNil(cloudMgr)
	assert.NoError(err)
	assert.IsType(cloudMgr, &dockerManager{})

	cloudMgr, err = GetContainerManager(ctx, "bogus", testutil.TestConfig())
	assert.Nil(cloudMgr)
	assert.EqualError(err, "No known container provider for 'bogus'")
}
