package services

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDialCedarOptionsValidate(t *testing.T) {
	t.Run("NoUsername", func(t *testing.T) {
		opts := &DialCedarOptions{
			BaseAddress: "base",
			RPCPort:     "9090",
			Password:    "password",
			Retries:     10,
		}
		assert.Error(t, opts.validate())
	})
	t.Run("NoPassword", func(t *testing.T) {
		opts := &DialCedarOptions{
			BaseAddress: "base",
			RPCPort:     "9090",
			Username:    "username",
			Retries:     10,
		}
		assert.Error(t, opts.validate())
	})
	t.Run("NoRPCPort", func(t *testing.T) {
		opts := &DialCedarOptions{
			BaseAddress: "base",
			Username:    "username",
			Password:    "password",
			Retries:     10,
		}
		assert.Error(t, opts.validate())
	})
	t.Run("DefaultBaseAddressAndClient", func(t *testing.T) {
		opts := &DialCedarOptions{
			Username: "username",
			Password: "password",
			Retries:  10,
		}
		assert.NoError(t, opts.validate())
		assert.Equal(t, "cedar.mongodb.com", opts.BaseAddress)
		assert.Equal(t, "7070", opts.RPCPort)
	})
	t.Run("ConfiguredOptions", func(t *testing.T) {
		opts := &DialCedarOptions{
			BaseAddress: "base",
			RPCPort:     "9090",
			Username:    "username",
			Password:    "password",
			Retries:     10,
		}
		assert.NoError(t, opts.validate())
		assert.Equal(t, "base", opts.BaseAddress)
		assert.Equal(t, "9090", opts.RPCPort)
		assert.Equal(t, "username", opts.Username)
		assert.Equal(t, "password", opts.Password)
		assert.Equal(t, 10, opts.Retries)
	})
}

func TestDialCedar(t *testing.T) {
	ctx := context.TODO()
	username := os.Getenv("LDAP_USER")
	password := os.Getenv("LDAP_PASSWORD")

	t.Run("ConnectToCedar", func(t *testing.T) {
		opts := &DialCedarOptions{
			Username: username,
			Password: password,
			Retries:  10,
		}
		conn, err := DialCedar(ctx, http.DefaultClient, opts)
		require.NoError(t, err)
		require.NotNil(t, conn)
		assert.NoError(t, conn.Close())
	})
	t.Run("IncorrectBaseAddress", func(t *testing.T) {
		opts := &DialCedarOptions{
			BaseAddress: "cedar.mongo.com",
			RPCPort:     "7070",
			Username:    username,
			Password:    password,
		}
		conn, err := DialCedar(ctx, http.DefaultClient, opts)
		assert.Error(t, err)
		assert.Nil(t, conn)
	})
	t.Run("IncorrectUsernameAndPassword", func(t *testing.T) {
		opts := &DialCedarOptions{
			Username: "bad_user",
			Password: "bad_password",
			Retries:  10,
		}
		conn, err := DialCedar(ctx, http.DefaultClient, opts)
		assert.Error(t, err)
		assert.Nil(t, conn)
	})
}
