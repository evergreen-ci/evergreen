package agent

import (
	"context"
	"os"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
)

func TestGetSenderRemote(t *testing.T) {
	assert := assert.New(t)
	_ = os.Setenv("GRIP_SUMO_ENDPOINT", "http://www.example.com/")
	_ = os.Setenv("GRIP_SPLUNK_SERVER_URL", "http://www.example.com/")
	_ = os.Setenv("GRIP_SPLUNK_CLIENT_TOKEN", "token")
	_ = os.Setenv("GRIP_SPLUNK_CHANNEL", "channel")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := GetSender(ctx, evergreen.LocalLoggingOverride, "task_id")
	assert.NoError(err)
}

func TestGetSenderLocal(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := GetSender(ctx, evergreen.LocalLoggingOverride, "task_id")
	assert.NoError(err)
}
