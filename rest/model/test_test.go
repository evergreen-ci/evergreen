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
					Status:   ToStringPtr("testStatus"),
					TestFile: ToStringPtr("testFile"),
					Logs: TestLogs{
						URL:     ToStringPtr("testUrl"),
						LineNum: 15,
						URLRaw:  ToStringPtr("testUrlRaw"),
						LogId:   ToStringPtr(""),
					},
					ExitCode:  1,
					StartTime: sTime,
					EndTime:   eTime,
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
					StartTime: time.Unix(0, 0),
					EndTime:   time.Unix(0, 0),
				},
				st: testresult.TestResult{},
			},
		}

		Convey("running BuildFromService(), should produce the equivalent model", func() {
			for _, tc := range modelPairs {
				apiTest := &APITest{}
				err := apiTest.BuildFromService(&tc.st)
				So(err, ShouldBeNil)
				So(FromStringPtr(apiTest.TestFile), ShouldEqual, FromStringPtr(tc.at.TestFile))
			}
		})
	})
}
