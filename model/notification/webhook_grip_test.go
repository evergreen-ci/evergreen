package notification

import (
	"bytes"
	"crypto/hmac"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
)

func TestEvergreenWebhookComposer(t *testing.T) {
	assert := assert.New(t)

	m := NewWebhookMessage("", nil, nil, nil)
	assert.False(m.Loggable())

	url, err := url.Parse("https://example.com")
	assert.NoError(err)

	m2, ok := NewWebhookMessage("evergreen", url, []byte("hi"), []byte("something important")).(*evergreenWebhookMessage)
	assert.True(ok)
	assert.Equal("evergreen", m2.id)
	assert.Equal(url, m2.url)
	assert.Equal("hi", string(m2.secret))
	assert.Equal("something important", string(m2.body))
}

func TestEvergreenWebhookSender(t *testing.T) {
	assert := assert.New(t)

	url, err := url.Parse("https://example.com")
	assert.NoError(err)
	assert.NotNil(url)

	m := NewWebhookMessage("evergreen", url, []byte("hi"), []byte("something important"))
	assert.True(m.Loggable())
	assert.NotNil(m)

	transport := mockTransport{
		secret: []byte("hi"),
	}

	sender, err := NewEvergreenWebhookLogger()
	assert.NoError(err)
	assert.NotNil(sender)

	s, ok := sender.(*evergreenWebhookLogger)
	assert.True(ok)
	s.client = &http.Client{
		Transport: &transport,
	}

	s.SetErrorHandler(func(err error, _ message.Composer) {
		t.Error("error handler was called, but shouldn't have been")
		t.FailNow()
	})
	s.Send(m)

	// unloggable message shouldn't send
	m = NewWebhookMessage("", nil, nil, nil)
	s.Send(m)
}

type mockTransport struct {
	lastUrl string
	secret  []byte
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       ioutil.NopCloser(nil),
	}
	if req.Method != http.MethodPost {
		resp.StatusCode = http.StatusMethodNotAllowed
		resp.Body = ioutil.NopCloser(bytes.NewBufferString(fmt.Sprintf("expected method POST, got %s", req.Method)))

		return resp, nil
	}

	mid := req.Header.Get(evergreenNotificationIDHeader)
	if len(mid) == 0 {
		resp.Body = ioutil.NopCloser(bytes.NewBufferString("no message id"))
		return resp, nil
	}

	sig := []byte(req.Header.Get(evergreenHMACHeader))
	if len(sig) == 0 {
		resp.StatusCode = http.StatusBadRequest
		resp.Body = ioutil.NopCloser(bytes.NewBufferString("signature is empty"))
		return resp, nil
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		resp.StatusCode = http.StatusInternalServerError
		resp.Body = ioutil.NopCloser(bytes.NewBufferString(err.Error()))
		return resp, nil
	}

	hash, err := util.CalculateHMACHash(t.secret, body)
	if err != nil {
		resp.StatusCode = http.StatusInternalServerError
		resp.Body = ioutil.NopCloser(bytes.NewBufferString(err.Error()))
		return resp, nil
	}

	if !hmac.Equal([]byte(hash), sig) {
		resp.StatusCode = http.StatusUnauthorized
		resp.Body = ioutil.NopCloser(bytes.NewBufferString(fmt.Sprintf("expected signature: %s, got %s", sig, hash)))
		return resp, nil
	}
	resp.StatusCode = http.StatusNoContent
	grip.Info(message.Fields{
		"message":   fmt.Sprintf("received %s", mid),
		"signature": string(sig),
		"body":      string(body),
	})

	return resp, nil
}
