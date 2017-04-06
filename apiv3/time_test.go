package apiv3

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTimeMarshal(t *testing.T) {
	Convey("When checking time", t, func() {
		utcLoc, err := time.LoadLocation("")
		So(err, ShouldBeNil)
		TestTimeAsString := "\"2017-04-06T19:53:46.404Z\""
		TestTimeAsTime := time.Date(2017, time.April, 6, 19, 53, 46,
			404000000, utcLoc)
		Convey("then time in same time zone should marshal correctly", func() {
			t := NewTime(TestTimeAsTime)
			res, err := json.Marshal(t)
			So(err, ShouldBeNil)
			So(string(res), ShouldEqual, TestTimeAsString)
		})
		Convey("then time in different time zone should marshal correctly", func() {
			offsetZone := time.FixedZone("testZone", 10)

			t := NewTime(TestTimeAsTime.In(offsetZone))
			res, err := json.Marshal(t)
			So(err, ShouldBeNil)
			So(string(res), ShouldEqual, TestTimeAsString)
		})

	})

}

func TestTimeUnmarshal(t *testing.T) {
	Convey("When checking time", t, func() {
		utcLoc, err := time.LoadLocation("")
		So(err, ShouldBeNil)
		TestTimeAsString := "\"2017-04-06T19:53:46.404Z\""
		TestTimeAsTime := time.Date(2017, time.April, 6, 19, 53, 46,
			404000000, utcLoc)
		Convey("then time should unmarshal correctly", func() {
			data := []byte(TestTimeAsString)
			res := APITime{}
			err := json.Unmarshal(data, &res)
			So(err, ShouldBeNil)
			t := NewTime(TestTimeAsTime)
			So(res, ShouldResemble, t)
		})
	})

}
