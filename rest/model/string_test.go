package model

import (
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/smartystreets/goconvey/convey/reporting"
)

func init() {
	reporting.QuietMode()
}

func TestStringMarshal(t *testing.T) {
	Convey("When checking string", t, func() {
		Convey("and marshalling", func() {
			Convey("then empty string should produce an empty string as output", func() {
				str := ToStringPtr("")
				data, err := json.Marshal(str)
				So(err, ShouldBeNil)
				So(string(data), ShouldEqual, "\"\"")
			})
			Convey("then non empty string should produce regular output", func() {
				testStr := "testStr"
				str := ToStringPtr(testStr)
				data, err := json.Marshal(str)
				So(err, ShouldBeNil)
				So(string(data), ShouldEqual, fmt.Sprintf("\"%s\"", testStr))
			})

		})
		Convey("and unmarshalling", func() {
			Convey("then non empty string should produce regular output", func() {
				var res *string
				testStr := "testStr"
				str := fmt.Sprintf("\"%s\"", testStr)
				err := json.Unmarshal([]byte(str), &res)
				So(err, ShouldBeNil)
				So(FromStringPtr(res), ShouldEqual, testStr)
			})
		})
	})
}
