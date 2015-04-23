package elb

import (
	"encoding/xml"
	"io/ioutil"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func mustReadFixture(t *testing.T, name string) []byte {
	b, e := ioutil.ReadFile("fixtures/" + name)
	if e != nil {
		t.Fatal("fixture " + name + " does not exist")
	}
	return b
}

func TestElb(t *testing.T) {
	Convey("Elb", t, func() {
		Convey("Marshalling", func() {
			f := mustReadFixture(t, "describe_load_balancers.xml")
			rsp := &DescribeLoadBalancersResponse{}
			e := xml.Unmarshal(f, rsp)
			So(e, ShouldBeNil)
			lbs := rsp.LoadBalancers
			So(len(lbs), ShouldEqual, 1)
			lb := lbs[0]
			So(lb.LoadBalancerName, ShouldEqual, "MyLoadBalancer")
			So(lb.CreatedTime.Unix(), ShouldEqual, 1369430131)
			So(lb.CanonicalHostedZoneName, ShouldEqual, "MyLoadBalancer-123456789.us-east-1.elb.amazonaws.com")
			So(len(lb.AvailabilityZones), ShouldEqual, 1)
			So(lb.AvailabilityZones[0], ShouldEqual, "us-east-1a")
			So(len(lb.Subnets), ShouldEqual, 0)
			So(lb.HealthCheckTarget, ShouldEqual, "HTTP:80/")
			So(lb.HealthCheckInterval, ShouldEqual, 90)
			So(len(lb.Listeners), ShouldEqual, 1)
			So(lb.SourceSecurityGroupOwnerAlias, ShouldEqual, "amazon-elb")
			listener := lb.Listeners[0]
			So(listener.Protocol, ShouldEqual, "HTTP")

			So(len(lb.Instances), ShouldEqual, 1)
			So(lb.Instances[0], ShouldEqual, "i-e4cbe38d")

		})

		Convey("MarshalInstanceHealth", func() {
			f := mustReadFixture(t, "describe_instances_health.xml")
			rsp := &DescribeInstanceHealthResponse{}
			e := xml.Unmarshal(f, rsp)
			So(e, ShouldBeNil)
			So(rsp, ShouldNotBeNil)
			So(len(rsp.InstanceStates), ShouldEqual, 1)

			state := rsp.InstanceStates[0]
			So(state.Description, ShouldEqual, "Instance registration is still in progress.")
			So(state.InstanceId, ShouldEqual, "i-315b7e51")

		})
	})
}
