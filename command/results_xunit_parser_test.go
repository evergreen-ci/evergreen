package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestXMLParsing(t *testing.T) {
	cwd := testutil.GetDirectoryOfFile()

	Convey("With some test xml files", t, func() {
		Convey("with a basic test junit file", func() {
			file, err := os.Open(filepath.Join(cwd, "testdata", "xunit", "junit_1.xml"))
			require.NoError(t, err, "Error reading file")
			defer file.Close()

			Convey("the file should parse without error", func() {
				res, err := parseXMLResults(file)
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
			file, err := os.Open(filepath.Join(cwd, "testdata", "xunit", "junit_2.xml"))
			require.NoError(t, err, "Error reading file")
			defer file.Close()

			Convey("the file should parse without error", func() {
				res, err := parseXMLResults(file)
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
			file, err := os.Open(filepath.Join(cwd, "testdata", "xunit", "junit_3.xml"))
			require.NoError(t, err, "Error reading file")
			defer file.Close()

			Convey("the file should parse without error", func() {
				res, err := parseXMLResults(file)
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
		Convey(`with a "real" java driver xunit file`, func() {
			file, err := os.Open(filepath.Join(cwd, "testdata", "xunit", "junit_4.xml"))
			require.NoError(t, err, "Error reading file")
			defer file.Close()

			Convey("the file should parse without error", func() {
				res, err := parseXMLResults(file)
				So(err, ShouldBeNil)
				So(len(res), ShouldBeGreaterThan, 0)

				Convey("and have proper values decoded", func() {
					So(res[0].Errors, ShouldEqual, 0)
					So(res[0].Failures, ShouldEqual, 0)
					So(res[0].Name, ShouldEqual, "com.mongodb.operation.InsertOperationSpecification")
					So(res[0].TestCases[0].Name, ShouldEqual, "should return correct result")
					So(res[0].SysOut, ShouldEqual, "out message")
					So(res[0].SysErr, ShouldEqual, "error message")
				})
			})
		})

		Convey("with a result file produced by a mocha junit reporter", func() {
			file, err := os.Open(filepath.Join(cwd, "testdata", "xunit", "mocha.xml"))
			require.NoError(t, err, "Error reading file")
			defer file.Close()

			Convey("the file should parse without error", func() {
				res, err := parseXMLResults(file)
				So(err, ShouldBeNil)
				So(len(res), ShouldEqual, 187)

				Convey("and have proper values decoded (spot checking here)", func() {
					So(res[0].Failures, ShouldEqual, 0)
					So(res[0].Time, ShouldEqual, 0)
					So(res[0].Tests, ShouldEqual, 0)
					So(res[0].Name, ShouldEqual, "Root Suite")
					So(res[1].Failures, ShouldEqual, 0)
					So(res[1].Time, ShouldEqual, 0.003)
					So(res[1].Tests, ShouldEqual, 4)
					So(res[1].Name, ShouldEqual, "bundles/common/components/AuthExpired/AuthExpired")
				})
			})
		})

		Convey("with a result file with errors", func() {
			file, err := os.Open(filepath.Join(cwd, "testdata", "xunit", "results.xml"))
			require.NoError(t, err, "Error reading file")
			defer file.Close()

			Convey("the file should parse without error", func() {
				res, err := parseXMLResults(file)
				So(err, ShouldBeNil)
				So(len(res), ShouldEqual, 1)

				Convey("and have proper values decoded", func() {
					So(res[0].Failures, ShouldEqual, 0)
					So(res[0].Time, ShouldEqual, 0.001)
					So(res[0].Tests, ShouldEqual, 2)
					So(res[0].Name, ShouldEqual, "unittest.loader.ModuleImportFailure-20170406180545")
					So(res[0].Errors, ShouldEqual, 2)
					So(res[0].TestCases[0].ClassName, ShouldEqual, "unittest.loader.ModuleImportFailure.tests")
					So(res[0].TestCases[0].Error, ShouldNotBeNil)
				})
			})
		})

		Convey("with a result file with test suite errors", func() {
			file, err := os.Open(filepath.Join(cwd, "testdata", "xunit", "junit_5.xml"))
			require.NoError(t, err, "Error reading file")
			defer file.Close()

			Convey("the file should parse without error", func() {
				res, err := parseXMLResults(file)
				So(err, ShouldBeNil)
				So(len(res), ShouldEqual, 1)

				Convey("and have proper values decoded", func() {
					So(res[0].Errors, ShouldEqual, 1)
					So(len(res[0].TestCases), ShouldEqual, 0)
					So(res[0].Error.Type, ShouldEqual, "java.lang.ExceptionInInitializerError")
					So(res[0].Error.Content, ShouldStartWith, "java.lang.ExceptionInInitializerError")
				})
			})
		})
	})
}

func TestXMLToModelConversion(t *testing.T) {
	Convey("With a parsed XML file and a task", t, func() {
		file, err := os.Open(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "xunit", "junit_3.xml"))
		require.NoError(t, err, "Error reading file")
		defer file.Close()
		res, err := parseXMLResults(file)
		So(err, ShouldBeNil)
		So(len(res), ShouldBeGreaterThan, 0)
		testTask := &task.Task{Id: "TEST", Execution: 5}

		Convey("when converting the results to model struct", func() {
			tests := []task.TestResult{}
			logs := []*model.TestLog{}
			for _, testCase := range res[0].TestCases {
				test, log := testCase.toModelTestResultAndLog(testTask)
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
