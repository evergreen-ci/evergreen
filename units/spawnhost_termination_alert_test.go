package units

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnhostAlertJob(t *testing.T) {
	assert := assert.New(t)
	config := testutil.TestConfig()
	db.SetGlobalSessionProvider(config.SessionFactory())
	testutil.HandleTestingErr(db.ClearCollections(user.Collection), t, "error clearing collections")
	assert.NoError(evergreen.UpdateConfig(config))
	u := user.DBUser{
		Id:           "me",
		EmailAddress: "something@example.com",
	}
	assert.NoError(u.Insert())

	j := makeSpawnhostTerminationAlertJob()
	j.User = u.Id
	j.Host = "someHost"
	internalSender := send.MakeInternalLogger()
	j.sender = internalSender
	ctx := context.Background()
	j.Run(ctx)
	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

	require.New(t).True(internalSender.HasMessage())
	msg := internalSender.GetMessage().Message.String()
	assert.Contains(msg, fmt.Sprintf(emailBody, j.Host))
}
