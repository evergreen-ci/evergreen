package cloud

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestGetManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	Convey("GetManager() should return non-nil for all valid provider names", t, func() {

		Convey("EC2OnDemand should be returned for ec2-ondemand provider name", func() {
			mgrOpts := ManagerOpts{Provider: evergreen.ProviderNameEc2OnDemand, ProviderKey: "key", ProviderSecret: "secret"}
			cloudMgr, err := GetManager(ctx, env, mgrOpts)
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &ec2Manager{})
		})

		Convey("Static should be returned for static provider name", func() {
			mgrOpts := ManagerOpts{Provider: evergreen.ProviderNameStatic, ProviderKey: "key", ProviderSecret: "secret"}
			cloudMgr, err := GetManager(ctx, env, mgrOpts)
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &staticManager{})
		})

		Convey("Mock should be returned for mock provider name", func() {
			mgrOpts := ManagerOpts{Provider: evergreen.ProviderNameMock, ProviderKey: "key", ProviderSecret: "secret"}
			cloudMgr, err := GetManager(ctx, env, mgrOpts)
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &mockManager{})
		})

		Convey("Invalid provider names should return nil with err", func() {
			mgrOpts := ManagerOpts{Provider: "bogus", ProviderKey: "key", ProviderSecret: "secret"}
			cloudMgr, err := GetManager(ctx, env, mgrOpts)
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
	assert.Error(err)
	assert.Nil(cm2)
}
