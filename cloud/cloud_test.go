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
			mgrOpts := ManagerOpts{Provider: "ec2", Region: ""}
			cloudMgr, err := GetManager(ctx, mgrOpts, testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &ec2Manager{})
		})

		Convey("EC2Spot should be returned for ec2-spot provider name", func() {
			mgrOpts := ManagerOpts{Provider: "ec2-spot", Region: ""}
			cloudMgr, err := GetManager(ctx, mgrOpts, testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &ec2Manager{})
		})

		Convey("EC2 should be returned for ec2 provider name", func() {
			mgrOpts := ManagerOpts{Provider: "ec2-ondemand", Region: ""}
			cloudMgr, err := GetManager(ctx, mgrOpts, testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &ec2Manager{})
		})

		Convey("Static should be returned for static provider name", func() {
			mgrOpts := ManagerOpts{Provider: "static", Region: ""}
			cloudMgr, err := GetManager(ctx, mgrOpts, testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &staticManager{})
		})

		Convey("Mock should be returned for mock provider name", func() {
			mgrOpts := ManagerOpts{Provider: "mock", Region: ""}
			cloudMgr, err := GetManager(ctx, mgrOpts, testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &mockManager{})
		})

		Convey("Invalid provider names should return nil with err", func() {
			mgrOpts := ManagerOpts{Provider: "bogus", Region: ""}
			cloudMgr, err := GetManager(ctx, mgrOpts, testutil.TestConfig())
			So(cloudMgr, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})

}

func TestConvertContainerManager(t *testing.T) {
	assert := assert.New(t)

	m1 := &dockerManager{}
	m2 := &staticManager{}

	cm1, err := ConvertContainerManager(m1)
	assert.NoError(err)
	assert.IsType(&dockerManager{}, cm1)

	cm2, err := ConvertContainerManager(m2)
	assert.EqualError(err, "Error converting manager to container manager")
	assert.Nil(cm2)

}
