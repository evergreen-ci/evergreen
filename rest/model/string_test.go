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
			Convey("then empty string should produce null as output", func() {
				str := APIString("")
				data, err := json.Marshal(&str)
				So(err, ShouldBeNil)
				So(string(data), ShouldEqual, "null")
			})
			Convey("then non empty string should produce regular output", func() {
				testStr := "testStr"
				str := APIString(testStr)
				data, err := json.Marshal(str)
				So(err, ShouldBeNil)
				So(string(data), ShouldEqual, fmt.Sprintf("\"%s\"", testStr))
			})

		})
		Convey("and unmarshalling", func() {
			Convey("then null should produce null as output", func() {
				var res APIString
				err := json.Unmarshal([]byte("null"), &res)
				So(err, ShouldBeNil)
				So(res, ShouldEqual, "")
			})
			Convey("then non empty string should produce regular output", func() {
				var res APIString
				testStr := "testStr"
				str := fmt.Sprintf("\"%s\"", testStr)
				err := json.Unmarshal([]byte(str), &res)
				So(err, ShouldBeNil)
				So(res, ShouldEqual, testStr)

			})

		})

	})

}
