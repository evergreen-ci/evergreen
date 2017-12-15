package operations

import (
	"errors"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/urfave/cli"
)

const (
	prettyStringFormat = "%-25s %-15s %-40s%-40s %-40s %-40s \n"
	timeFormat         = "2006-01-02T15:04:05"
	csvFormat          = "csv"
	prettyFormat       = "pretty"
	jsonFormat         = "json"
)

func TestHistory() cli.Command {
	const (
		taskFlagName       = "task"
		testFlagName       = "test"
		taskStatusFlagName = "task-status"
		testStatusFlagName = "test-status"
		beforeRevFlagName  = "before-revision"
		afterRevFlagName   = "after-revision"
		beforeDateFlagName = "before-date"
		afterDateFlagName  = "after-date"
		earliestFlagName   = "earliest"
		formatFlagName     = "format"
		limitFlagName      = "limit"
		requestFlagName    = "request-source"
	)

	return cli.Command{
		Name:  "test-history",
		Usage: "",
		Flags: addProjectFlag(addOutputPath(
			cli.StringSliceFlag{
				Name:  taskFlagName,
				Usage: "task name",
			},
			cli.StringSliceFlag{
				Name:  testFlagName,
				Usage: "test name",
			},
			cli.StringSliceFlag{
				Name:  taskStatusFlagName,
				Usage: "task status, either fail, pass, sysfail, or timeout ",
			},
			cli.StringSliceFlag{
				Name:  testStatusFlagName,
				Usage: "test status, either fail, silentfail, pass, skip, or timeout",
			},
			cli.StringFlag{
				Name:  beforeRevFlagName,
				Usage: "find tests that finished before a full revision hash (40 characters) (inclusive)",
			},
			cli.StringFlag{
				Name:  afterRevFlagName,
				Usage: "find tests that finished after a full revision hash (40 characters) (exclusive)",
			},
			cli.StringFlag{
				Name:  beforeDateFlagName,
				Usage: "find tests that finished before a date in format YYYY-MM-DDTHH:MM:SS in UTC",
			},
			cli.StringFlag{
				Name:  afterDateFlagName,
				Usage: "find tests that finish after a date in format YYYY-MM-DDTHH:MM:SS in UTC",
			},
			cli.BoolFlag{
				Name:  earliestFlagName,
				Usage: "sort test history from the earliest revisions to latest",
			},
			cli.StringFlag{
				Name:  formatFlagName,
				Value: prettyFormat,
				Usage: "format to export test history, options are 'json', 'csv', 'pretty'",
			},
			cli.IntFlag{
				Name:  limitFlagName,
				Usage: "number of tasks to include the request. defaults to no limit, but you must specify either a limit or before/after revisions",
			},
			cli.StringFlag{
				Name:    requestFlagName,
				Aliases: []string{"r"},
				Usage:   "include 'patch', 'commit' or 'all' builds. Only shows commit builds if not specified",
			})),
		Before: mergeBeforeFuncs(
			requireStringLengthIfSpecified(beforeRevFlagName, 40),
			requireStringLengthIfSpecified(afterRevFlagName, 40),
			requireStringValueChoices(requestFlagName, []string{"patch", "commit", "all"}),
			requireStringValueChoices(formatFlagName, []string{csvFormat, jsonFormat, prettyFormat}),
			requireStringValueChoices(taskStatusFlagName, []string{"pass", "fail", "silentfail", "skip", "timeout"}),
			requireStringValueChoices(testStatusFlagName, []string{"pass", "fail", "sysfail", "timeout"}),
			func(c *cli.Context) error {
				if c.String(formatFlagName) != prettyFormat && c.String(pathFlagName) {
					return errors.New("must specify a filepath for csv and json output")
				}
				return nil
			},
			func(c *cli.Context) error {
				if c.Int(limitFlagName) == 0 {
					if c.String(beforeRevFlagName) == "" || c.String(afterRevFlagName) {
						return errors.New("must specify either a limit or before/after revision")
					}
				}
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			beforeDateArg := c.String(beforeDateFlagName)
			afterDateArg := c.String(afterDateFlagName)
			format := c.String(formatFlagName)
			outputPath := c.String(pathFlagName)

			// parse dates into time.Time values
			beforeDate := time.Time{}
			if beforeDateArg != "" {
				beforeDate, err = time.Parse(timeFormat, beforeDateArg)
				if err != nil {
					return errors.Errorf("before date should have format YYYY-MM-DDTHH:MM:SS, error: %v", err)
				}
			}

			afterDate := time.Time{}
			if beforeDateArg != "" {
				afterDate, err = time.Parse(timeFormat, afterDateArg)
				if err != nil {
					return errors.Errorf("after date should have format YYYY-MM-DDTHH:MM:SS, error: %v", err)
				}
			}
			earliest := c.Bool(earliestFlagName)

			// create a test history parameter struct and validate it
			testHistoryParameters := model.TestHistoryParameters{
				Project:         c.String(projectFlagName),
				TaskNames:       c.StringSlice(taskFlagName),
				TestNames:       c.StringSlice(testFlagName),
				BuildVariants:   c.StringSlice(variantsFlagName),
				TaskStatuses:    testHistoryGetTaskStatuses(c.StringSlice(taskStatusFlagName)),
				TestStatuses:    testHistoryGetTestStatuses(c.StringSlice(testStatusFlagName)),
				BeforeRevision:  c.String(beforeRevFlagName),
				AfterRevision:   c.String(afterRevFlagName),
				BeforeDate:      beforeDate,
				AfterDate:       afterDate,
				Sort:            -1,
				Limit:           c.Int(limit),
				TaskRequestType: c.String(requestFlagName),
			}

			if earliest {
				testHistoryParameters.Sort = 1
			}

			// process request

			if err = testHistoryParameters.SetDefaultsAndValidate(); err != nil {
				return err
			}

			// set up clients
			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			_, rc, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			isCSV := false
			if format == csvFormat {
				isCSV = true
			}
			body, err := rc.GetTestHistory(testHistoryParameters.Project, testHistoryParameters.QueryString(), isCSV)
			if err != nil {
				return err
			}
			defer body.Close()

			if format == prettyFormat {
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

			return util.WriteToFile(body, outputPath)

		},
	}
}

func testHistoryGetTaskStatuses(stats []string) []string {
	taskStatuses := []string{}
	for _, s := range stats {
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
	return taskStatuses
}

func testHistoryGetTestStatuses(stats []string) []string {
	testStatuses := []string{}
	for _, s := range stats {
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
	return testStatuses
}
