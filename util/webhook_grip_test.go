package util

import (
	"bytes"
	"crypto/hmac"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
)

func TestEvergreenWebhookComposer(t *testing.T) {
	assert := assert.New(t)

	m := NewWebhookMessage("", "", nil, nil)
	assert.False(m.Loggable())

	url := "https://example.com"

	m2, ok := NewWebhookMessage("evergreen", url, []byte("hi"), []byte("something important")).(*evergreenWebhookMessage)
	assert.True(ok)
	assert.Equal("evergreen", m2.raw.NotificationID)
	assert.Equal(url, m2.raw.URL)
	assert.Equal("hi", string(m2.raw.Secret))
	assert.Equal("something important", string(m2.raw.Body))
}

func TestEvergreenWebhookSender(t *testing.T) {
	assert := assert.New(t)

	m := NewWebhookMessage("evergreen", "https://example.com", []byte("hi"), []byte("something important"))
	assert.True(m.Loggable())
	assert.NotNil(m)

	transport := mockWebhookTransport{
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
	m = NewWebhookMessage("", "", nil, nil)
	s.Send(m)
}

func TestEvergreenWebhookSenderWithBadSecret(t *testing.T) {
	assert := assert.New(t)

	m := NewWebhookMessage("evergreen", "https://example.com", []byte("bye"), []byte("something forged"))
	assert.True(m.Loggable())
	assert.NotNil(m)

	transport := mockWebhookTransport{
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

	channel := make(chan error, 1)
	s.SetErrorHandler(func(err error, _ message.Composer) {
		channel <- err
	})
	s.Send(m)

	assert.EqualError(<-channel, "evergreen-webhook response status was 400 Bad Request")
}

type mockWebhookTransport struct {
	lastUrl string
	secret  []byte
}

func (t *mockWebhookTransport) RoundTrip(req *http.Request) (*http.Response, error) {
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

	hash, err := CalculateHMACHash(t.secret, body)
	if err != nil {
		resp.StatusCode = http.StatusInternalServerError
		resp.Body = ioutil.NopCloser(bytes.NewBufferString(err.Error()))
		return resp, nil
	}

	if !hmac.Equal([]byte(hash), sig) {
		resp.StatusCode = http.StatusBadRequest
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
