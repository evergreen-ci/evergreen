package xunit

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"testing"
)

func TestXMLParsing(t *testing.T) {
	Convey("With some test xml files", t, func() {
		Convey("with a basic test junit file", func() {
			file, err := os.Open("testdata/junit_1.xml")
			testutil.HandleTestingErr(err, t, "Error reading file")

			Convey("the file should parse without error", func() {
				res, err := ParseXMLResults(file)
				So(err, ShouldBeNil)
				So(len(res), ShouldBeGreaterThan, 0)

				Convey("and have proper values decoded", func() {
					So(res[0].Errors, ShouldEqual, 1)
					So(res[0].Failures, ShouldEqual, 5)
					So(res[0].Name, ShouldEqual, "nose2-junit")
					So(res[0].TestCases[11].Name, ShouldEqual, "test_params_func:2")
					So(res[0].TestCases[11].Time, ShouldEqual, 0.000098)
					So(res[0].TestCases[11].Failure, ShouldNotBeNil)
					So(res[0].TestCases[11].Failure.Message, ShouldEqual, "test failure")
				})
			})
		})

		Convey("with a more complex test junit file", func() {
			file, err := os.Open("testdata/junit_2.xml")
			testutil.HandleTestingErr(err, t, "Error reading file")

			Convey("the file should parse without error", func() {
				res, err := ParseXMLResults(file)
				So(err, ShouldBeNil)
				So(len(res), ShouldBeGreaterThan, 0)

				Convey("and have proper values decoded", func() {
					So(res[0].Errors, ShouldEqual, 1)
					So(res[0].Failures, ShouldEqual, 1)
					So(res[0].Name, ShouldEqual, "tests.ATest")
					So(res[0].SysOut, ShouldContainSubstring, "here")
					So(res[0].TestCases[0].Name, ShouldEqual, "error")
					So(res[0].TestCases[0].ClassName, ShouldEqual, "tests.ATest")
					So(res[0].TestCases[0].Time, ShouldEqual, 0.0060)
					So(res[0].TestCases[0].Failure, ShouldBeNil)
					So(res[0].TestCases[0].Error, ShouldNotBeNil)
					So(res[0].TestCases[0].Error.Type, ShouldEqual, "java.lang.RuntimeException")

				})
			})
		})

		Convey(`with a "real" pymongo xunit file`, func() {
			file, err := os.Open("testdata/junit_3.xml")
			testutil.HandleTestingErr(err, t, "Error reading file")

			Convey("the file should parse without error", func() {
				res, err := ParseXMLResults(file)
				So(err, ShouldBeNil)
				So(len(res), ShouldBeGreaterThan, 0)

				Convey("and have proper values decoded", func() {
					So(res[0].Errors, ShouldEqual, 0)
					So(res[0].Failures, ShouldEqual, 1)
					So(res[0].Skip, ShouldEqual, 188)
					So(res[0].Name, ShouldEqual, "nosetests")
					So(res[0].TestCases[0].Name, ShouldEqual, "test_uri_options")
					So(res[0].TestCases[0].ClassName, ShouldEqual, "test.test_auth.TestAuthURIOptions")
					So(res[0].TestCases[0].Time, ShouldEqual, 0.002)
					So(res[0].TestCases[0].Failure, ShouldBeNil)
					So(res[0].TestCases[0].Error, ShouldBeNil)
					So(res[0].TestCases[0].Skipped, ShouldNotBeNil)
					So(res[0].TestCases[0].Skipped.Type, ShouldEqual, "unittest.case.SkipTest")
					So(res[0].TestCases[0].Skipped.Content, ShouldContainSubstring,
						"SkipTest: Authentication is not enabled on server")
				})
			})
		})
	})
}

func TestXMLToModelConversion(t *testing.T) {
	Convey("With a parsed XML file and a task", t, func() {
		file, err := os.Open("testdata/junit_3.xml")
		testutil.HandleTestingErr(err, t, "Error reading file")
		res, err := ParseXMLResults(file)
		So(err, ShouldBeNil)
		So(len(res), ShouldBeGreaterThan, 0)
		testTask := &task.Task{Id: "TEST", Execution: 5}

		Convey("when converting the results to model struct", func() {
			tests := []task.TestResult{}
			logs := []*model.TestLog{}
			for _, testCase := range res[0].TestCases {
				test, log := testCase.ToModelTestResultAndLog(testTask)
				if log != nil {
					logs = append(logs, log)
				}
				tests = append(tests, test)
			}

			Convey("the proper amount of each failure should be correct", func() {
				skipCount := 0
				failCount := 0
				passCount := 0
				for _, t := range tests {
					switch t.Status {
					case evergreen.TestFailedStatus:
						failCount++
					case evergreen.TestSkippedStatus:
						skipCount++
					case evergreen.TestSucceededStatus:
						passCount++
					}
				}

				So(failCount, ShouldEqual, res[0].Failures+res[0].Errors)
				So(skipCount, ShouldEqual, res[0].Skip)
				//make sure we didn't miss anything
				So(passCount+skipCount+failCount, ShouldEqual, len(tests))

				Convey("and logs should be of the proper form", func() {
					So(logs[0].Name, ShouldNotEqual, "")
					So(len(logs[0].Lines), ShouldNotEqual, 0)
					So(logs[0].URL(), ShouldContainSubstring,
						"TEST/5/test.test_auth.TestAuthURIOptions.test_uri_options")
				})
			})
		})
	})
}
