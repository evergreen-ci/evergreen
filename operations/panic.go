package operations

import (
	"context"
	"fmt"
	"os"

	"github.com/evergreen-ci/evergreen/rest/model"
)

// SendPanicReport sends a panic report to the Evergreen service.
func SendPanicReport(ctx context.Context, report *model.PanicReport) error {
	if report == nil || report.ConfigFilePath == "" {
		fmt.Fprintln(os.Stderr, "unexpected error occured, could not reach out to Evergreen service:", report.Panic)
		os.Exit(1)
	}

	conf, err := NewClientSettings(report.ConfigFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unexpected error occured, could not load config file '%s': %v\n", report.ConfigFilePath, err)
		os.Exit(1)
	}

	comm, err := conf.setupRestCommunicator(ctx, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unexpected error occured, could not set up REST communicator: %v\n", err)
		os.Exit(1)
	}
	defer comm.Close()

	return comm.SendPanicReport(ctx, report)
}
