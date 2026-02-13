package operations

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/rest/model"
)

// SendPanicReport sends a panic report to the Evergreen service.
func SendPanicReport(ctx context.Context, report *model.PanicReport) error {
	if report == nil || report.ConfigFilePath == "" {
		return fmt.Errorf("could not reach Evergreen service: %v", report.Panic)
	}

	conf, err := NewClientSettings(report.ConfigFilePath)
	if err != nil {
		return fmt.Errorf("could not load config file '%s': %v", report.ConfigFilePath, err)
	}

	comm, err := conf.setupRestCommunicator(ctx, false)
	if err != nil {
		return fmt.Errorf("could not set up REST communicator: %v", err)
	}
	defer comm.Close()

	return comm.SendPanicReport(ctx, report)
}
