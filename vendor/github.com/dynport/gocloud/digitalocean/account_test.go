package digitalocean

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAccount(t *testing.T) {
	Convey("Account", t, func() {
		account := &Account{RegionId: 1122, SizeId: 10}
		So(account, ShouldNotBeNil)
		droplet := account.DefaultDroplet()
		So(droplet, ShouldNotBeNil)
		So(account.RegionId, ShouldEqual, 1122)
		So(account.SizeId, ShouldEqual, 10)
	})
}
