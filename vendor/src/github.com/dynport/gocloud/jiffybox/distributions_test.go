package jiffybox

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDistributions(t *testing.T) {
	Convey("Distributions", t, func() {
		f := mustReadFixture(t, "distributions.json")
		rsp := &DistributionsResponse{}
		e := json.Unmarshal(f, rsp)
		So(e, ShouldBeNil)
		So(rsp, ShouldNotBeNil)

		So(len(rsp.DistributionsMap), ShouldEqual, 2)
		So(len(rsp.Distributions()), ShouldEqual, 2)

		dist := rsp.Distributions()[0]
		So(dist.Name, ShouldEqual, "CentOS 5.4")
		So(dist.Key, ShouldEqual, "centos_5_4_32bit")

	})
}
