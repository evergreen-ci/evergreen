package units

import (
	"context"
	"testing"

	"github.com/mongodb/jasper"
)

func startMockJasperService(ctx context.Context) (jasper.CloseFunc, error) {
	return nil, nil
}

func TestJasperDeployJob(t *testing.T) {
	// Populates unset variables
	// Generates new Jasper variables
	// Sends credentials file write command
	// Gets ID
	// Sends restart Jasper command
	// Gets ID again
}
