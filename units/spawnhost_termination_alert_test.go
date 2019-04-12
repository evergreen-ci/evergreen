package units

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnhostAlertJob(t *testing.T) {
	assert := assert.New(t)
	config := testutil.TestConfig()
	db.SetGlobalSessionProvider(config.SessionFactory())
	assert.NoError(db.ClearCollections(user.Collection), t, "error clearing collections")
	assert.NoError(evergreen.UpdateConfig(config))
	u := user.DBUser{
		Id:           "me",
		EmailAddress: "something@example.com",
	}
	assert.NoError(u.Insert())

	j := makeSpawnhostTerminationAlertJob()
	j.User = u.Id
	j.Host = "someHost"
	ctx := context.Background()
	env := &mock.Environment{}
	j.env = env
	assert.NoError(env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))

	j.Run(ctx)
	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

	require.New(t).True(env.InternalSender.HasMessage())
	msg := env.InternalSender.GetMessage().Message.String()
	assert.Contains(msg, fmt.Sprintf(emailBody, j.Host))
}
