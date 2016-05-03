package util

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCappedWriter(t *testing.T) {
	Convey("With a new CappedWriter with a max of 10 bytes", t, func() {
		cw := CappedWriter{
			Buffer:   &bytes.Buffer{},
			MaxBytes: 10,
		}

		Convey("a write of size 8 should succeed", func() {
			n, err := cw.Write([]byte("01234567"))
			So(n, ShouldEqual, 8)
			So(err, ShouldBeNil)
			So(cw.IsFull(), ShouldBeFalse)
			Convey("and a subsequent write of 2 should succeed", func() {
				n, err := cw.Write([]byte("89"))
				So(n, ShouldEqual, 2)
				So(err, ShouldBeNil)
				So(cw.String(), ShouldEqual, "0123456789")
			})
			Convey("but a write of 3 should fail", func() {
				n, err := cw.Write([]byte("89A"))
				So(err, ShouldEqual, ErrBufferFull)
				Convey("but still write 2/3", func() {
					So(n, ShouldEqual, 2)
					So(cw.String(), ShouldEqual, "0123456789")
				})
			})
		})
		Convey("a write of size 10 should succeed", func() {
			n, err := cw.Write([]byte("0123456789"))
			So(n, ShouldEqual, 10)
			So(err, ShouldBeNil)
		})
		Convey("a write of size 12 should error", func() {
			n, err := cw.Write([]byte("0123456789AB"))
			So(err, ShouldEqual, ErrBufferFull)
			Convey("but still write 10/12", func() {
				So(n, ShouldEqual, 10)
				So(cw.String(), ShouldEqual, "0123456789")
				So(cw.IsFull(), ShouldBeTrue)
			})
		})
	})
}
