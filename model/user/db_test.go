package user

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
)

func TestUpsertOneFromExisting(t *testing.T) {
	assert.NoError(t, db.Clear(Collection))
	oldUsr := &DBUser{
		Id:           "hello.world",
		EmailAddress: "hello.world@classic.com",
		APIKey:       "veryKey",
		PubKeys: []PubKey{
			{Name: "pub", Key: "key"},
		},
		SystemRoles: []string{"admin"},
		Settings: UserSettings{
			Region: "here",
		},
		FavoriteProjects: []string{"evergreen"},
		PatchNumber:      12,
	}
	newUsr, err := UpsertOneFromExisting(t.Context(), oldUsr, "hello.howareyou@adele.com")
	assert.NoError(t, err)
	assert.Equal(t, "hello.howareyou", newUsr.Id)
	assert.Equal(t, "hello.howareyou@adele.com", newUsr.Email())
	assert.Equal(t, oldUsr.APIKey, newUsr.APIKey)
	assert.Equal(t, oldUsr.PubKeys, newUsr.PubKeys)
	assert.Equal(t, oldUsr.SystemRoles, newUsr.SystemRoles)
	assert.Equal(t, oldUsr.Settings, newUsr.Settings)
	assert.Equal(t, oldUsr.FavoriteProjects, newUsr.FavoriteProjects)
	assert.Equal(t, oldUsr.PatchNumber, newUsr.PatchNumber)

	// Ensure this also works when the person has already logged in as their new user.
	existingUsr := &DBUser{
		Id:          "newly.created",
		APIKey:      "autogenerated",
		SystemRoles: []string{"different"},
		PatchNumber: 1,
	}
	assert.NoError(t, db.Insert(t.Context(), Collection, existingUsr))
	newUsr, err = UpsertOneFromExisting(t.Context(), oldUsr, "newly.created@new.com")
	assert.NoError(t, err)
	assert.Equal(t, "newly.created", newUsr.Id)
	assert.Equal(t, "newly.created@new.com", newUsr.Email())
	assert.Equal(t, oldUsr.APIKey, newUsr.APIKey)
	assert.Equal(t, oldUsr.PubKeys, newUsr.PubKeys)
	assert.Equal(t, oldUsr.SystemRoles, newUsr.SystemRoles)
	assert.Equal(t, oldUsr.Settings, newUsr.Settings)
	assert.Equal(t, oldUsr.FavoriteProjects, newUsr.FavoriteProjects)
	assert.Equal(t, oldUsr.PatchNumber, newUsr.PatchNumber)

}
