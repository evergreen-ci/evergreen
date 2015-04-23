package s3

import (
	. "github.com/smartystreets/goconvey/convey"
	"net/url"
	"testing"
)

func TestSignRequest(t *testing.T) {
	Convey("Sign request", t, func() {
		u, e := url.Parse("http://127.0.0.1/?uploads&a=1")
		if e != nil {
			t.Fatal(e.Error())
		}
		v := normalizeParams(u)
		So(v, ShouldEqual, "uploads")
	})
}
