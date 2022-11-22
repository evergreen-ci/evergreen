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

	m := NewWebhookMessage(EvergreenWebhook{})
	assert.False(m.Loggable())

	url := "https://example.com"
	header := http.Header{
		"test": []string{},
	}
	m = NewWebhookMessage(EvergreenWebhook{
		NotificationID: "evergreen",
		URL:            url,
		Secret:         []byte("hi"),
		Body:           []byte("something important"),
		Headers:        header,
	})
	assert.False(m.Loggable())

	header = http.Header{
		"Test": []string{"test1", "test2"},
	}
	m = NewWebhookMessage(EvergreenWebhook{
		NotificationID: "evergreen",
		URL:            url,
		Secret:         []byte("hi"),
		Body:           []byte("something important"),
		Headers:        header,
	})
	assert.True(m.Loggable())
	m2, ok := m.(*evergreenWebhookMessage)
	assert.True(ok)
	assert.Equal("evergreen", m2.raw.NotificationID)
	assert.Equal(url, m2.raw.URL)
	assert.Equal("hi", string(m2.raw.Secret))
	assert.Equal("something important", string(m2.raw.Body))
	assert.Len(m2.raw.Headers, 1)
	assert.Len(m2.raw.Headers["Test"], 2)
	assert.Contains(m2.raw.Headers["Test"], "test1")
	assert.Contains(m2.raw.Headers["Test"], "test2")

	m = NewWebhookMessage(EvergreenWebhook{
		NotificationID: "evergreen",
		URL:            url,
		Secret:         []byte("hi"),
		Body:           []byte("something important"),
		Headers:        nil,
	})
	assert.True(m.Loggable())
}

func TestEvergreenWebhookSender(t *testing.T) {
	assert := assert.New(t)

	header := http.Header{
		"test": []string{"test1", "test2"},
	}

	m := NewWebhookMessage(EvergreenWebhook{
		NotificationID: "evergreen",
		URL:            "https://example.com",
		Secret:         []byte("hi"),
		Body:           []byte("something important"),
		Headers:        header,
	})
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

	assert.NoError(s.SetErrorHandler(func(err error, _ message.Composer) {
		t.Error("error handler was called, but shouldn't have been")
		t.FailNow()
	}))
	s.Send(m)
	assert.Equal("https://example.com", transport.lastUrl)

	assert.Len(transport.header, 3)
	assert.Len(transport.header["Test"], 2)
	assert.Contains(transport.header["Test"], "test1")
	assert.Contains(transport.header["Test"], "test2")

	// unloggable message shouldn't send
	m = NewWebhookMessage(EvergreenWebhook{})
	s.Send(m)
	assert.NotEqual("", transport.lastUrl)
}

func TestEvergreenWebhookSenderWithBadSecret(t *testing.T) {
	assert := assert.New(t)

	m := NewWebhookMessage(EvergreenWebhook{
		NotificationID: "evergreen",
		URL:            "https://example.com",
		Secret:         []byte("bye"),
		Body:           []byte("something forged"),
		Headers:        nil,
	})
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
	assert.NoError(s.SetErrorHandler(func(err error, _ message.Composer) {
		channel <- err
	}))
	s.Send(m)

	assert.EqualError(<-channel, "response was 400 (Bad Request)")
	assert.Equal("https://example.com", transport.lastUrl)
}

type mockWebhookTransport struct {
	lastUrl string
	secret  []byte
	header  http.Header
}

func (t *mockWebhookTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.lastUrl = req.URL.String()
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
	t.header = req.Header

	return resp, nil
}
