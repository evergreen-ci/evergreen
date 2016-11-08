package route53

import (
	"encoding/xml"
	"io/ioutil"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSerialize(t *testing.T) {
	req := NewChangeResourceRecordSets(
		&ChangeBatch{
			Comment: "This is a comment",
			Changes: []*Change{
				{Action: "CREATE"},
			},
		},
	)
	Convey("Serialize", t, func() {
		b, e := xml.Marshal(req)
		So(e, ShouldBeNil)
		So(b, ShouldNotBeNil)
		s := string(b)
		So(s, ShouldContainSubstring, "xmlns")
		So(s, ShouldContainSubstring, "<Changes>")
		So(s, ShouldContainSubstring, "<ChangeBatch>")
		So(s, ShouldContainSubstring, "<Change>")
	})
}

func mustReadFixture(name string) []byte {
	b, e := ioutil.ReadFile("fixtures/" + name)
	if e != nil {
		panic(e.Error())
	}
	return b
}

func TestParseHostedZones(t *testing.T) {
	Convey("ParseHostedZones", t, func() {
		f := mustReadFixture("list_hosted_zones.xml")
		rsp := &ListHostedZonesResponse{}
		e := xml.Unmarshal(f, rsp)
		So(e, ShouldBeNil)
		So(len(rsp.HostedZones), ShouldEqual, 1)

		So(rsp.MaxItems, ShouldEqual, 1)
		So(rsp.IsTruncated, ShouldEqual, true)

		zone := rsp.HostedZones[0]
		So(zone.Id, ShouldEqual, "/hostedzone/Z111111QQQQQQQ")
		So(zone.Name, ShouldEqual, "example2.com.")
		So(zone.CallerReference, ShouldEqual, "MyUniqueIdentifier2")
		So(zone.ResourceRecordSetCount, ShouldEqual, 42)

	})
}

func TestParseGetHostedZOneResponse(t *testing.T) {
	Convey("ParseGetHostedZoneResponse", t, func() {
		f := mustReadFixture("get_hosted_zone_response.xml")
		rsp := &GetHostedZoneResponse{}
		e := xml.Unmarshal(f, rsp)
		So(e, ShouldBeNil)
		zone := rsp.HostedZone

		So(e, ShouldBeNil)

		So(zone.Id, ShouldEqual, "/hostedzone/Z1PA6795UKMFR9")
		So(zone.Name, ShouldEqual, "example.com.")

		nameServers := rsp.NameServers
		So(len(nameServers), ShouldEqual, 4)
		So(nameServers[0], ShouldEqual, "ns-2048.awsdns-64.com")

	})
}

func TestListResourceRecordSets(t *testing.T) {
	Convey("ListResourceRecordSets", t, func() {
		f := mustReadFixture("list_resource_record_sets.xml")
		rsp := &ListResourceRecordSetsResponse{}
		e := xml.Unmarshal(f, rsp)
		So(e, ShouldBeNil)
		So(rsp, ShouldNotBeNil)
		So(len(rsp.ResourceRecordSets), ShouldEqual, 1)

		rrs := rsp.ResourceRecordSets[0]
		So(rrs.Name, ShouldEqual, "example.com.")
		So(rrs.Type, ShouldEqual, "NS")
		So(rrs.TTL, ShouldEqual, 172800)

		So(len(rrs.ResourceRecords), ShouldEqual, 4)

		resourceRecord := rrs.ResourceRecords[0]
		So(resourceRecord, ShouldNotBeNil)
		So(resourceRecord.Value, ShouldEqual, "ns-2048.awsdns-64.com.")

	})
}
