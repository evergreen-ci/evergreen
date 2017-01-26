package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/util"
)

var (
	earliestSort = "earliest"
	latestSort   = "latest"

	jsonFormat   = "json"
	csvFormat    = "csv"
	prettyFormat = "pretty"
)

const (
	prettyStringFormat = "%-25s %-15s %-15s%-20s %-15s %-20s \n"
)

// TestHistoryCommand represents the test-history command in the CLI
type TestHistoryCommand struct {
	GlobalOpts *Options `no-flag:"true"`

	Project        string    `long:"project" short:"p" description:"project identifier, defaults to user's default project"`
	Tasks          []string  `long:"task" description:"task name"`
	Tests          []string  `long:"test" description:"test name"`
	Variants       []string  `long:"variant" short:"v" description:"variant name"`
	TaskStatuses   []string  `long:"task-status" description:"task status, either fail, pass, sysfail, or timeout "`
	TestStatuses   []string  `long:"test-status" description:"test status, either fail, pass, skip, or timeout "`
	BeforeRevision string    `long:"before-revision" description:"find tests that finished before a full revision hash (40 characters) (inclusive)"`
	AfterRevision  string    `long:"after-revision" description:"find tests that finished after a full revision hash (40 characters) (exclusive)"`
	BeforeDate     time.Time `long:"before-date" description:"find tests that finished before a date in format YYYY-MM-DDTHH:MM:SS"`
	AfterDate      time.Time `long:"after-date" description:"find tests that finish after a date in format YYYY-MM-DDTHH:MM:SS"`
	Earliest       bool      `long:"earliest" description:"sort test history from the earliest revisions to latest"`
	Filepath       string    `long:"filepath" description:"path to directory where file is to be saved, only used with json or csv format"`
	Format         string    `long:"format" description:"format to export test history, options are 'json', 'csv', 'pretty', default pretty to stdout"`
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
		queryString += fmt.Sprintf("&beforeDate=%v", testHistoryParameters.BeforeDate)
	}
	if !util.IsZeroTime(testHistoryParameters.AfterDate) {
		queryString += fmt.Sprintf("&afterDate=%v", testHistoryParameters.AfterDate)
	}
	fmt.Println(queryString)
	return queryString
}

// Execute transfers the fields from a TestHistoryCommand to a TestHistoryParameter
// and validates them. It then gets the test history from the api endpoint
func (thc *TestHistoryCommand) Execute(args []string) error {
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
		return fmt.Errorf("after revision must be a 40 character revision")
	}

	if thc.BeforeRevision != "" && len(thc.BeforeRevision) != 40 {
		return fmt.Errorf("before revision must be a 40 character revision")
	}

	if thc.Format == "" {
		thc.Format = prettyFormat
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
		BeforeDate:     thc.BeforeDate,
		AfterDate:      thc.AfterDate,
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
		util.ReadJSONInto(body, &results)
		fmt.Printf(prettyStringFormat, "Start Time", "Duration(ms)", "Variant", "TaskName", "TestName", "URL")
		for _, thr := range results {
			formattedStart := thr.StartTime.Format(time.ANSIC)
			fmt.Printf(prettyStringFormat, formattedStart, thr.DurationMS, thr.BuildVariant,
				thr.TaskName, thr.TestFile, thr.Url)
		}
		return nil
	}

	return WriteToFile(body, thc.Filepath)

}
