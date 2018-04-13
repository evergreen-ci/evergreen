package units

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
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
	j.UseMock = true
	ctx := context.Background()
	j.Run(ctx)
	assert.NoError(j.Error())

	assert.Len(j.sentMails, 1)
	assert.Equal(fmt.Sprintf(emailBody, "someHost"), j.sentMails[0].Body)
	assert.Equal(emailSubject, j.sentMails[0].Subject)
	assert.Equal(u.EmailAddress, j.sentMails[0].Recipients[0])
}
