package ec2

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRunInstancesConfig(t *testing.T) {
	Convey("Values", t, func() {
		So(1, ShouldEqual, 1)
		c := &RunInstancesConfig{ImageId: "image-id", SubnetId: "subnet"}
		c.BlockDeviceMappings = []*BlockDeviceMapping{
			{
				DeviceName: "/dev/something",
				Ebs: &Ebs{
					VolumeSize: 50,
					VolumeType: VolumeTypeGp,
				},
			},
		}
		c.AddPublicIp()
		v, e := c.Values()
		So(e, ShouldBeNil)
		So(v, ShouldNotBeNil)
		So(v.Get("BlockDeviceMapping.0.DeviceName"), ShouldEqual, "/dev/something")
		So(v.Get("BlockDeviceMapping.0.Ebs.VolumeSize"), ShouldEqual, "50")
		So(v.Get("BlockDeviceMapping.0.Ebs.VolumeType"), ShouldEqual, "gp2")
		So(len(v), ShouldEqual, 9)
	})
}
