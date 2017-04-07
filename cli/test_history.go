package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

var ()

const (
	prettyStringFormat = "%-25s %-15s %-40s%-40s %-40s %-40s \n"
	timeFormat         = "2006-01-02T15:04:05"
	csvFormat          = "csv"
	prettyFormat       = "pretty"
	// jsonFormat   = "json" // not used
)

// TestHistoryCommand represents the test-history command in the CLI
type TestHistoryCommand struct {
	GlobalOpts *Options `no-flag:"true"`

	Project        string   `long:"project" short:"p" description:"project identifier, defaults to user's default project"`
	Tasks          []string `long:"task" description:"task name"`
	Tests          []string `long:"test" description:"test name"`
	Variants       []string `long:"variant" short:"v" description:"variant name"`
	TaskStatuses   []string `long:"task-status" description:"task status, either fail, pass, sysfail, or timeout "`
	TestStatuses   []string `long:"test-status" description:"test status, either fail, pass, skip, or timeout "`
	BeforeRevision string   `long:"before-revision" description:"find tests that finished before a full revision hash (40 characters) (inclusive)"`
	AfterRevision  string   `long:"after-revision" description:"find tests that finished after a full revision hash (40 characters) (exclusive)"`
	// TODO EVG-1540 for user specific timezones.
	BeforeDate string `long:"before-date" description:"find tests that finished before a date in format YYYY-MM-DDTHH:MM:SS in UTC"`
	AfterDate  string `long:"after-date" description:"find tests that finish after a date in format YYYY-MM-DDTHH:MM:SS in UTC"`
	Earliest   bool   `long:"earliest" description:"sort test history from the earliest revisions to latest"`
	Filepath   string `long:"filepath" description:"path to directory where file is to be saved, only used with json or csv format"`
	Format     string `long:"format" description:"format to export test history, options are 'json', 'csv', 'pretty', default pretty to stdout"`
}

// createUrlQuery returns a string url query parameter with relevant url parameters.
func createUrlQuery(testHistoryParameters model.TestHistoryParameters) string {
	queryString := fmt.Sprintf("testStatuses=%v&taskStatuses=%v", strings.Join(testHistoryParameters.TestStatuses, ","),
		strings.Join(testHistoryParameters.TaskStatuses, ","))

	if len(testHistoryParameters.TaskNames) > 0 {
		queryString += fmt.Sprintf("&tasks=%v", strings.Join(testHistoryParameters.TaskNames, ","))
	}

	if len(testHistoryParameters.TestNames) > 0 {
		queryString += fmt.Sprintf("&tests=%v", strings.Join(testHistoryParameters.TestNames, ","))
	}

	if len(testHistoryParameters.BuildVariants) > 0 {
		queryString += fmt.Sprintf("&variants=%v", strings.Join(testHistoryParameters.BuildVariants, ","))
	}

	if testHistoryParameters.BeforeRevision != "" {
		queryString += fmt.Sprintf("&beforeRevision=%v", testHistoryParameters.BeforeRevision)
	}

	if testHistoryParameters.AfterRevision != "" {
		queryString += fmt.Sprintf("&afterRevision=%v", testHistoryParameters.AfterRevision)
	}
	if !util.IsZeroTime(testHistoryParameters.BeforeDate) {
		queryString += fmt.Sprintf("&beforeDate=%v", testHistoryParameters.BeforeDate.Format(time.RFC3339))
	}
	if !util.IsZeroTime(testHistoryParameters.AfterDate) {
		queryString += fmt.Sprintf("&afterDate=%v", testHistoryParameters.AfterDate.Format(time.RFC3339))
	}
	return queryString
}

// Execute transfers the fields from a TestHistoryCommand to a TestHistoryParameter
// and validates them. It then gets the test history from the api endpoint
func (thc *TestHistoryCommand) Execute(_ []string) error {
	_, rc, _, err := getAPIClients(thc.GlobalOpts)
	if err != nil {
		return err
	}

	// convert the test and tasks statuses to the correct evergreen statuses
	testStatuses := []string{}
	for _, s := range thc.TestStatuses {
		switch s {
		case "pass":
			testStatuses = append(testStatuses, evergreen.TestSucceededStatus)
		case "fail":
			testStatuses = append(testStatuses, evergreen.TestFailedStatus)
		case "skip":
			testStatuses = append(testStatuses, evergreen.TestSkippedStatus)
		case "timeout":
			testStatuses = append(testStatuses, model.TaskTimeout)
		}
	}

	taskStatuses := []string{}
	for _, s := range thc.TaskStatuses {
		switch s {
		case "pass":
			taskStatuses = append(taskStatuses, evergreen.TaskSucceeded)
		case "fail":
			taskStatuses = append(taskStatuses, evergreen.TaskFailed)
		case "sysfail":
			taskStatuses = append(taskStatuses, model.TaskSystemFailure)
		case "timeout":
			taskStatuses = append(taskStatuses, model.TaskTimeout)
		}
	}

	sort := -1
	if thc.Earliest {
		sort = 1
	}

	if thc.AfterRevision != "" && len(thc.AfterRevision) != 40 {
		return errors.Errorf("after revision must be a 40 character revision")
	}

	if thc.BeforeRevision != "" && len(thc.BeforeRevision) != 40 {
		return errors.Errorf("before revision must be a 40 character revision")
	}

	if thc.Format == "" {
		thc.Format = prettyFormat
	}
	beforeDate := time.Time{}
	if thc.BeforeDate != "" {
		beforeDate, err = time.Parse(timeFormat, thc.BeforeDate)
		if err != nil {
			return errors.Errorf("before date should have format YYYY-MM-DDTHH:MM:SS, error: %v", err)
		}
	}
	afterDate := time.Time{}
	if thc.AfterDate != "" {
		afterDate, err = time.Parse(timeFormat, thc.AfterDate)
		if err != nil {
			return errors.Errorf("after date should have format YYYY-MM-DDTHH:MM:SS, error: %v", err)
		}
	}
	// create a test history parameter struct and validate it
	testHistoryParameters := model.TestHistoryParameters{
		Project:        thc.Project,
		TaskNames:      thc.Tasks,
		TestNames:      thc.Tests,
		BuildVariants:  thc.Variants,
		TaskStatuses:   taskStatuses,
		TestStatuses:   testStatuses,
		BeforeRevision: thc.BeforeRevision,
		AfterRevision:  thc.AfterRevision,
		BeforeDate:     beforeDate,
		AfterDate:      afterDate,
		Sort:           sort,
	}

	if err := testHistoryParameters.SetDefaultsAndValidate(); err != nil {
		return err
	}
	isCSV := false
	if thc.Format == csvFormat {
		isCSV = true
	}
	body, err := rc.GetTestHistory(testHistoryParameters.Project, createUrlQuery(testHistoryParameters), isCSV)
	if err != nil {
		return err
	}
	defer body.Close()

	if thc.Format == prettyFormat {
		results := []service.RestTestHistoryResult{}

		if err := util.ReadJSONInto(body, &results); err != nil {
			return err
		}

		fmt.Printf(prettyStringFormat, "Start Time", "Duration(ms)", "Variant", "Task Name", "Test File", "URL")
		for _, thr := range results {
			if !util.IsZeroTime(thr.StartTime) {
				formattedStart := thr.StartTime.Format(time.ANSIC)
				fmt.Printf(prettyStringFormat, formattedStart, thr.DurationMS, thr.BuildVariant,
					thr.TaskName, thr.TestFile, thr.Url)
			}
		}
		return nil
	}

	return WriteToFile(body, thc.Filepath)

}
