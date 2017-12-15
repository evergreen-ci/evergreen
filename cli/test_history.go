package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

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
	TestStatuses   []string `long:"test-status" description:"test status, either fail, silentfail, pass, skip, or timeout "`
	BeforeRevision string   `long:"before-revision" description:"find tests that finished before a full revision hash (40 characters) (inclusive)"`
	AfterRevision  string   `long:"after-revision" description:"find tests that finished after a full revision hash (40 characters) (exclusive)"`
	// TODO EVG-1540 for user specific timezones.
	BeforeDate string `long:"before-date" description:"find tests that finished before a date in format YYYY-MM-DDTHH:MM:SS in UTC"`
	AfterDate  string `long:"after-date" description:"find tests that finish after a date in format YYYY-MM-DDTHH:MM:SS in UTC"`
	Earliest   bool   `long:"earliest" description:"sort test history from the earliest revisions to latest"`
	Filepath   string `long:"filepath" description:"path to directory where file is to be saved, only used with json or csv format"`
	Format     string `long:"format" description:"format to export test history, options are 'json', 'csv', 'pretty', default pretty to stdout"`
	Limit      int    `long:"limit" description:"number of tasks to include the request. defaults to no limit, but you must specify either a limit or before/after revisions."`
	Request    string `long:"request-source" short:"r" description:"include 'patch', 'commit' or 'all' builds. Only shows commit builds if not specified."`
}

// Execute transfers the fields from a TestHistoryCommand to a TestHistoryParameter
// and validates them. It then gets the test history from the api endpoint
func (thc *TestHistoryCommand) Execute(_ []string) error {
	if thc.Format == "" {
		thc.Format = prettyFormat
	}

	if thc.Format != prettyFormat && thc.Filepath == "" {
		return errors.New("must specify a filepath for csv and json output")
	}

	ctx := context.Background()
	_, rc, _, err := getAPIClients(ctx, thc.GlobalOpts)
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
		case "silentfail":
			testStatuses = append(testStatuses, evergreen.TestSilentlyFailedStatus)
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
		Project:         thc.Project,
		TaskNames:       thc.Tasks,
		TestNames:       thc.Tests,
		BuildVariants:   thc.Variants,
		TaskStatuses:    taskStatuses,
		TestStatuses:    testStatuses,
		BeforeRevision:  thc.BeforeRevision,
		AfterRevision:   thc.AfterRevision,
		BeforeDate:      beforeDate,
		AfterDate:       afterDate,
		Sort:            sort,
		Limit:           thc.Limit,
		TaskRequestType: thc.Request,
	}

	if err = testHistoryParameters.SetDefaultsAndValidate(); err != nil {
		return err
	}
	isCSV := false
	if thc.Format == csvFormat {
		isCSV = true
	}
	body, err := rc.GetTestHistory(testHistoryParameters.Project, testHistoryParameters.QueryString(), isCSV)
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

	return util.WriteToFile(body, thc.Filepath)
}
