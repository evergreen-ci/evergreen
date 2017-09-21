package gimlet

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/negroni"
)

func TestReqestLogger(t *testing.T) {
	assert := assert.New(t)

	sender, err := send.NewInternalLogger("test", grip.GetSender().Level())
	assert.NoError(err)
	middlewear := &AppLogging{
		Journaler: &logging.Grip{sender},
	}

	next := func(w http.ResponseWriter, r *http.Request) {
		middlewear.Journaler.Info("hello")
	}
	assert.False(sender.HasMessage())
	req := &http.Request{
		URL: &url.URL{},
	}
	rw := negroni.NewResponseWriter(nil)

	startAt := getNumber()
	middlewear.ServeHTTP(rw, req, next)
	assert.Equal(startAt+2, getNumber())
	assert.True(sender.HasMessage())
	assert.Equal(sender.Len(), 3)
}
