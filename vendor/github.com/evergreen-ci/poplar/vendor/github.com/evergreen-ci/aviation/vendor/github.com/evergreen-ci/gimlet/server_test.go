package gimlet

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	t.Run("AppValidation", func(t *testing.T) {
		conf := ServerConfig{
			Timeout: time.Minute,
			App:     NewApp(),
			Address: "foo:200",
		}

		assert.False(t, conf.handlerGenerated)
		require.NoError(t, conf.Validate())
		assert.True(t, conf.handlerGenerated)
	})
	t.Run("ServerWithTLSRequiresTLS", func(t *testing.T) {
		srv, err := BuildNewServer("example.com", nil, nil)
		require.Nil(t, srv)
		require.Error(t, err)
		require.Contains(t, err.Error(), "nil tls config")
	})
	t.Run("NewServer", func(t *testing.T) {
		srv, err := NewServer("1.2.3.4:80", nil)
		require.Nil(t, srv)
		require.Error(t, err)
		require.Contains(t, err.Error(), "must specify a handler")
	})
	t.Run("BuildNewServer", func(t *testing.T) {
		srv, err := BuildNewServer("1.2.3.4:80", nil, &tls.Config{Certificates: []tls.Certificate{tls.Certificate{}}})
		require.Nil(t, srv)
		require.Error(t, err)
		require.Contains(t, err.Error(), "must specify a handler")
	})
	t.Run("ServerIntegration", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		app := NewApp()
		h, err := app.Handler()
		require.NoError(t, err)
		srv, err := BuildNewServer("1.2.3.4:80", h, &tls.Config{Certificates: []tls.Certificate{tls.Certificate{}}})
		require.NotNil(t, srv)
		require.NoError(t, err)
		assert.NotNil(t, srv.GetServer())
		wait, err := srv.Run(ctx)
		require.NoError(t, err)
		require.NotNil(t, wait)
	})
}
