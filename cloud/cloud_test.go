package cloud

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	Convey("GetManager() should return non-nil for all valid provider names", t, func() {

		Convey("ec2-ondemand provider name should be rejected", func() {
			mgrOpts := ManagerOpts{Provider: evergreen.ProviderNameEc2OnDemand}
			cloudMgr, err := GetManager(ctx, env, mgrOpts)
			So(cloudMgr, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

		Convey("Static should be returned for static provider name", func() {
			mgrOpts := ManagerOpts{Provider: evergreen.ProviderNameStatic}
			cloudMgr, err := GetManager(ctx, env, mgrOpts)
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &staticManager{})
		})

		Convey("Mock should be returned for mock provider name", func() {
			mgrOpts := ManagerOpts{Provider: evergreen.ProviderNameMock}
			cloudMgr, err := GetManager(ctx, env, mgrOpts)
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &mockManager{})
		})

		Convey("Invalid provider names should return nil with err", func() {
			mgrOpts := ManagerOpts{Provider: "bogus"}
			cloudMgr, err := GetManager(ctx, env, mgrOpts)
			So(cloudMgr, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})

}
