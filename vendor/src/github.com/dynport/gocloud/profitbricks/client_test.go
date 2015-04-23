package profitbricks

import (
	"encoding/xml"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestMarshlling(t *testing.T) {
	req := &CreateStorageRequest{}
	b, e := xml.Marshal(req)
	Convey("serializing", t, func() {
		Convey("the error should not be nil", func() {
			So(e, ShouldBeNil)
		})
		Convey("the serialized xml should have the correct format", func() {
			So(string(b), ShouldStartWith, "<request>")
		})
	})
}

func TestMultiRequest(t *testing.T) {
	s := marshalMultiRequest("createStorage", &CreateServerRequest{DataCenterId: "Id"})
	Convey("serialize multi request", t, func() {
		So(s, ShouldContainSubstring, "<tns:createStorage><request>")
	})
}
