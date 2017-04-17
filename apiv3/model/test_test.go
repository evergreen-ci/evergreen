package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

type testCompare struct {
	at APITest
	st task.TestResult
}

func TestTestBuildFromService(t *testing.T) {
	Convey("With a list of models to compare", t, func() {
		sTime := time.Unix(12345, 6789)
		eTime := time.Unix(9876, 54321)
		modelPairs := []testCompare{

			{
				at: APITest{
					Status:   APIString("testStatus"),
					TestFile: APIString("testFile"),
					Logs: TestLogs{
						URL:     APIString("testUrl"),
						LineNum: 15,
						URLRaw:  APIString("testUrlRaw"),
						LogId:   "",
					},
					ExitCode:  1,
					StartTime: APITime(sTime),
					EndTime:   APITime(eTime),
				},
				st: task.TestResult{
					Status:    "testStatus",
					TestFile:  "testFile",
					URL:       "testUrl",
					URLRaw:    "testUrlRaw",
					LineNum:   15,
					LogId:     "",
					ExitCode:  1,
					StartTime: util.ToPythonTime(sTime),
					EndTime:   util.ToPythonTime(eTime),
				},
			},
			{
				at: APITest{
					StartTime: APITime(time.Unix(0, 0)),
					EndTime:   APITime(time.Unix(0, 0)),
				},
				st: task.TestResult{},
			},
		}

		Convey("running BuildFromService(), should produce the equivalent model", func() {
			for _, tc := range modelPairs {
				apiTest := &APITest{}
				err := apiTest.BuildFromService(&tc.st)
				So(err, ShouldBeNil)
				So(apiTest, ShouldResemble, &tc.at)
			}
		})
	})
}
