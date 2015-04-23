package ec2

import (
	"encoding/xml"
	"io/ioutil"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEc2(t *testing.T) {
	Convey("TestEc2", t, func() {
		So(1, ShouldEqual, 1)

		Convey("MarshalRunInstanceResponse", func() {
			rsp := &RunInstancesResponse{}
			e := xml.Unmarshal(mustReadFixture(t, "run_instances_response.xml"), rsp)
			So(e, ShouldBeNil)
			So(len(rsp.Instances), ShouldEqual, 1)
			i := rsp.Instances[0]
			So(i.InstanceId, ShouldEqual, "i-1122")

		})

		Convey("Marshalling", func() {
			f := mustReadFixture(t, "describe_images.xml")
			rsp := &DescribeImagesResponse{}
			e := xml.Unmarshal(f, rsp)
			So(e, ShouldBeNil)
			So(rsp.RequestId, ShouldEqual, "59dbff89-35bd-4eac-99ed-be587EXAMPLE")
			So(f, ShouldNotBeNil)
			So(len(rsp.Images), ShouldEqual, 1)

			img := rsp.Images[0]
			So(img.ImageId, ShouldEqual, "ami-1a2b3c4d")
		})

		Convey("MarshalTags", func() {
			f := mustReadFixture(t, "describe_tags.xml")
			rsp := &DescribeTagsResponse{}
			e := xml.Unmarshal(f, rsp)
			So(e, ShouldBeNil)
			So(len(rsp.Tags), ShouldEqual, 6)
			tag := rsp.Tags[1]
			So(tag.Key, ShouldEqual, "stack")
			So(tag.Value, ShouldEqual, "Production")

			tag = &Tag{Key: "Name", Value: "staging"}
			b, e := xml.Marshal(tag)
			So(e, ShouldBeNil)
			s := string(b)
			So(s, ShouldContainSubstring, "staging")
			//So(s, ShouldContainSubstring, "resourceId")
		},
		)

		//func TestDescribeKeyPair(t *testing.T) {
		//	f := mustReadFixture(t, "describe_key_pairs.xml")
		//	rsp := &DescribeKeyPairsResponse{}
		//	e := xml.Unmarshal(f, rsp)
		//	assert.Nil(t, e)
		//	assert.NotNil(t, rsp)
		//	assert.Equal(t, len(rsp.KeyPairs), 1)
		//
		//	pair := rsp.KeyPairs[0]
		//	assert.Equal(t, pair.KeyName, "my-key-pair")
		//	assert.Equal(t, pair.KeyFingerprint, "1f:51:ae:28:bf:89:e9:d8:1f:25:5d:37:2d:7d:b8:ca:9f:f5:f1:6f")
		//}
		//
		//func TestAddresses(t *testing.T) {
		//	f := mustReadFixture(t, "describe_addresses.xml")
		//	assert.NotNil(t, f)
		//	rsp := &DescribeAddressesResponse{}
		//	e := xml.Unmarshal(f, rsp)
		//	assert.Nil(t, e)
		//	assert.NotNil(t, rsp)
		//	assert.Equal(t, len(rsp.Addresses), 1)
		//	a := rsp.Addresses[0]
		//	assert.Equal(t, a.PublicIp, "203.0.113.41")
		//}
		//
		//func TestSecurityGroups(t *testing.T) {
		//	f := mustReadFixture(t, "describe_security_groups.xml")
		//	assert.NotNil(t, f)
		//	rsp := &DescribeSecurityGroupsResponse{}
		//	e := xml.Unmarshal(f, rsp)
		//	assert.Nil(t, e)
		//	assert.NotNil(t, rsp)
		//	assert.Equal(t, len(rsp.SecurityGroups), 2)
		//
		//	group := rsp.SecurityGroups[0]
		//	assert.Equal(t, group.GroupId, "sg-1a2b3c4d")
		//	assert.Equal(t, len(group.IpPermissions), 1)
		//
		//	perm := group.IpPermissions[0]
		//	assert.Equal(t, perm.IpProtocol, "tcp")
		//	assert.Equal(t, perm.FromPort, 80)
		//
		//	assert.Equal(t, len(perm.IpRanges), 1)
		//	assert.Equal(t, perm.IpRanges[0], "0.0.0.0/0")
		//
		//	add2 := rsp.SecurityGroups[1]
		//	assert.NotNil(t, add2)
		//	assert.Equal(t, len(add2.IpPermissions[0].Groups), 1)
		//}
	})
}

func mustReadFixture(t *testing.T, name string) []byte {
	b, e := ioutil.ReadFile("fixtures/" + name)
	if e != nil {
		t.Fatal("fixture " + name + " does not exist")
	}
	return b
}
