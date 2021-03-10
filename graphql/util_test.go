package graphql

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
)

func init() {
	testutil.Setup()
}

var testResults = []apimodels.CedarTestResult{
	apimodels.CedarTestResult{
		TestName: "A test",
		Status:   "Pass",
		Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
		End:      time.Date(1996, time.August, 31, 12, 5, 12, 0, time.UTC),
	},
	apimodels.CedarTestResult{
		TestName: "B test",
		Status:   "Fail",
		Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
		End:      time.Date(1996, time.August, 31, 12, 5, 16, 0, time.UTC),
	},
	apimodels.CedarTestResult{
		TestName: "C test",
		Status:   "Fail",
		Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
		End:      time.Date(1996, time.August, 31, 12, 5, 15, 0, time.UTC),
	},
	apimodels.CedarTestResult{
		TestName: "D test",
		Status:   "Pass",
		Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
		End:      time.Date(1996, time.August, 31, 12, 5, 11, 0, time.UTC),
	},
}

func TestFilterSortAndPaginateCedarTestResults(t *testing.T) {

	Convey("when Provided with no params", t, func() {
		Convey("the result should be the same", func() {
			result, count := FilterSortAndPaginateCedarTestResults(testResults, "", []string{}, "", 0, 0, 0)
			So(result, ShouldResemble, testResults)
			So(count, ShouldEqual, 4)
		})

	})
	Convey("when Provided with a status filter", t, func() {
		Convey("the result should only contain CedarTestResults with that status", func() {
			result, count := FilterSortAndPaginateCedarTestResults(testResults, "", []string{"Fail"}, "", 0, 0, 0)
			So(result, ShouldResemble, []apimodels.CedarTestResult{
				apimodels.CedarTestResult{
					TestName: "B test",
					Status:   "Fail",
					Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
					End:      time.Date(1996, time.August, 31, 12, 5, 16, 0, time.UTC),
				},
				apimodels.CedarTestResult{
					TestName: "C test",
					Status:   "Fail",
					Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
					End:      time.Date(1996, time.August, 31, 12, 5, 15, 0, time.UTC),
				},
			})
			So(count, ShouldEqual, 2)
		})
	})
	Convey("when Provided with a sort key", t, func() {
		Convey("the result should be sorted based off of that key", func() {
			Convey("by duration asc", func() {
				result, count := FilterSortAndPaginateCedarTestResults(testResults, "", []string{}, "duration", 1, 0, 0)
				So(result, ShouldResemble, []apimodels.CedarTestResult{
					apimodels.CedarTestResult{
						TestName: "D test",
						Status:   "Pass",
						Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
						End:      time.Date(1996, time.August, 31, 12, 5, 11, 0, time.UTC),
					},
					apimodels.CedarTestResult{
						TestName: "A test",
						Status:   "Pass",
						Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
						End:      time.Date(1996, time.August, 31, 12, 5, 12, 0, time.UTC),
					},
					apimodels.CedarTestResult{
						TestName: "C test",
						Status:   "Fail",
						Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
						End:      time.Date(1996, time.August, 31, 12, 5, 15, 0, time.UTC),
					},
					apimodels.CedarTestResult{
						TestName: "B test",
						Status:   "Fail",
						Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
						End:      time.Date(1996, time.August, 31, 12, 5, 16, 0, time.UTC),
					},
				})
				So(count, ShouldEqual, 4)
			})
			Convey("by testname asc", func() {
				result, count := FilterSortAndPaginateCedarTestResults(testResults, "", []string{}, testresult.TestFileKey, 1, 0, 0)
				So(result, ShouldResemble, []apimodels.CedarTestResult{
					apimodels.CedarTestResult{
						TestName: "A test",
						Status:   "Pass",
						Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
						End:      time.Date(1996, time.August, 31, 12, 5, 12, 0, time.UTC),
					},
					apimodels.CedarTestResult{
						TestName: "B test",
						Status:   "Fail",
						Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
						End:      time.Date(1996, time.August, 31, 12, 5, 16, 0, time.UTC),
					},
					apimodels.CedarTestResult{
						TestName: "C test",
						Status:   "Fail",
						Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
						End:      time.Date(1996, time.August, 31, 12, 5, 15, 0, time.UTC),
					},
					apimodels.CedarTestResult{
						TestName: "D test",
						Status:   "Pass",
						Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
						End:      time.Date(1996, time.August, 31, 12, 5, 11, 0, time.UTC),
					},
				})
				So(count, ShouldEqual, 4)
			})
			Convey("by status asc", func() {
				result, count := FilterSortAndPaginateCedarTestResults(testResults, "", []string{}, testresult.StatusKey, 1, 0, 0)
				So(result, ShouldResemble, []apimodels.CedarTestResult{

					apimodels.CedarTestResult{
						TestName: "B test",
						Status:   "Fail",
						Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
						End:      time.Date(1996, time.August, 31, 12, 5, 16, 0, time.UTC),
					},
					apimodels.CedarTestResult{
						TestName: "C test",
						Status:   "Fail",
						Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
						End:      time.Date(1996, time.August, 31, 12, 5, 15, 0, time.UTC),
					},
					apimodels.CedarTestResult{
						TestName: "A test",
						Status:   "Pass",
						Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
						End:      time.Date(1996, time.August, 31, 12, 5, 12, 0, time.UTC),
					},
					apimodels.CedarTestResult{
						TestName: "D test",
						Status:   "Pass",
						Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
						End:      time.Date(1996, time.August, 31, 12, 5, 11, 0, time.UTC),
					},
				})
				So(count, ShouldEqual, 4)
			})
		})
	})
	Convey("when Provided with a limit", t, func() {
		Convey("the result should be paginated", func() {
			result, count := FilterSortAndPaginateCedarTestResults(testResults, "", []string{}, "", 0, 0, 2)
			So(result, ShouldResemble, []apimodels.CedarTestResult{
				apimodels.CedarTestResult{
					TestName: "A test",
					Status:   "Pass",
					Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
					End:      time.Date(1996, time.August, 31, 12, 5, 12, 0, time.UTC),
				},
				apimodels.CedarTestResult{
					TestName: "B test",
					Status:   "Fail",
					Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
					End:      time.Date(1996, time.August, 31, 12, 5, 16, 0, time.UTC),
				},
			})
			So(count, ShouldEqual, 4)

			result, count = FilterSortAndPaginateCedarTestResults(testResults, "", []string{}, "", 0, 1, 2)
			So(result, ShouldResemble, []apimodels.CedarTestResult{
				apimodels.CedarTestResult{
					TestName: "C test",
					Status:   "Fail",
					Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
					End:      time.Date(1996, time.August, 31, 12, 5, 15, 0, time.UTC),
				},
				apimodels.CedarTestResult{
					TestName: "D test",
					Status:   "Pass",
					Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
					End:      time.Date(1996, time.August, 31, 12, 5, 11, 0, time.UTC),
				},
			})
			So(count, ShouldEqual, 4)
		})
	})

}
