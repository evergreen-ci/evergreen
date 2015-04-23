package jiffybox

import (
	"encoding/json"
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

func TestJiffyBoxes(t *testing.T) {
	Convey("JiffyBoxes", t, func() {

		f := mustReadFixture(t, "jiffyBoxes.json")

		rsp := &JiffyBoxesResponse{}
		e := json.Unmarshal(f, rsp)
		So(e, ShouldBeNil)
		So(len(rsp.Messages), ShouldEqual, 0)

		So(len(rsp.Servers()), ShouldEqual, 1)

		server := rsp.Server()
		So(server.Id, ShouldEqual, 12345)
		So(server.Name, ShouldEqual, "Test")
		So(len(server.Ips), ShouldEqual, 2)

		public := server.Ips["public"]

		So(public[0], ShouldEqual, "188.93.14.176")
		So(server.Status, ShouldEqual, "READY")

		plan := server.Plan
		So(plan.Id, ShouldEqual, 22)
		So(plan.Name, ShouldEqual, "CloudLevel 3")
		So(plan.RamInMB, ShouldEqual, 8192)

		So(server.Metadata["createdby"], ShouldEqual, "JiffyBoxTeam")
		ap := server.ActiveProfile
		So(ap.Name, ShouldEqual, "Standard")
		So(ap.Created, ShouldEqual, 1234567890)

		So(len(ap.DisksHash), ShouldEqual, 2)
		So(len(ap.Disks()), ShouldEqual, 2)

		disk := ap.DisksHash["xvda"]

		So(disk.Name, ShouldEqual, "CentOS 5.4")
		So(disk.SizeInMB, ShouldEqual, 81408)
	})
}

func TestUnmarshalling(t *testing.T) {
	Convey("Unmarshalling", t, func() {
		f := mustReadFixture(t, "error_creating_response.json")
		rsp := &ErrorResponse{}
		e := json.Unmarshal(f, rsp)
		So(e, ShouldBeNil)

		f = mustReadFixture(t, "no_module_response.json")
		rsp = &ErrorResponse{}
		e = json.Unmarshal(f, rsp)
		So(e, ShouldBeNil)
		t.Log(rsp.Result)
		So(rsp, ShouldNotBeNil)
	})

}
