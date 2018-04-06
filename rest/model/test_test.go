package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

type testCompare struct {
	at APITest
	st testresult.TestResult
}

func TestTestBuildFromService(t *testing.T) {
	Convey("With a list of models to compare", t, func() {
		sTime := time.Unix(12345, 6789)
		eTime := time.Unix(9876, 54321)
		modelPairs := []testCompare{

			{
				at: APITest{
					Status:   ToAPIString("testStatus"),
					TestFile: ToAPIString("testFile"),
					Logs: TestLogs{
						URL:     ToAPIString("testUrl"),
						LineNum: 15,
						URLRaw:  ToAPIString("testUrlRaw"),
						LogId:   ToAPIString(""),
					},
					ExitCode:  1,
					StartTime: NewTime(sTime),
					EndTime:   NewTime(eTime),
				},
				st: testresult.TestResult{
					Status:    "testStatus",
					TestFile:  "testFile",
					URL:       "testUrl",
					URLRaw:    "testUrlRaw",
					LineNum:   15,
					LogID:     "",
					ExitCode:  1,
					StartTime: util.ToPythonTime(sTime),
					EndTime:   util.ToPythonTime(eTime),
				},
			},
			{
				at: APITest{
					StartTime: NewTime(time.Unix(0, 0)),
					EndTime:   NewTime(time.Unix(0, 0)),
				},
				st: testresult.TestResult{},
			},
		}

		Convey("running BuildFromService(), should produce the equivalent model", func() {
			for _, tc := range modelPairs {
				apiTest := &APITest{}
				err := apiTest.BuildFromService(&tc.st)
				So(err, ShouldBeNil)
				So(FromAPIString(apiTest.TestFile), ShouldEqual, FromAPIString(tc.at.TestFile))
			}
		})
	})
}
