package util

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

const (
	defaultWebhookTimeout         = 10 * time.Second
	defaultMinDelay               = 500 * time.Millisecond
	evergreenNotificationIDHeader = "X-Evergreen-Notification-ID"
	evergreenHMACHeader           = "X-Evergreen-Signature"
)

type EvergreenWebhook struct {
	NotificationID string      `bson:"notification_id"`
	URL            string      `bson:"url"`
	Secret         []byte      `bson:"secret"`
	Body           []byte      `bson:"body"`
	Headers        http.Header `bson:"headers"`
	Retries        int         `bson:"retries"`
	MinDelayMS     int         `bson:"min_delay_ms"`
	TimeoutMS      int         `bson:"timeout_ms"`
}

type evergreenWebhookMessage struct {
	raw EvergreenWebhook

	message.Base
}

func NewWebhookMessage(raw EvergreenWebhook) message.Composer {
	return &evergreenWebhookMessage{
		raw: raw,
	}
}

func (w *evergreenWebhookMessage) Loggable() bool {
	if len(w.raw.NotificationID) == 0 {
		return false
	}
	if len(w.raw.Secret) == 0 {
		return false
	}
	if len(w.raw.Body) == 0 {
		return false
	}
	if len(w.raw.URL) == 0 {
		return false
	}
	for k := range w.raw.Headers {
		if len(w.raw.Headers[k]) == 0 {
			return false
		}
	}

	_, err := url.Parse(w.raw.URL)
	grip.Error(message.WrapError(err, message.Fields{
		"message":         "evergreen-webhook invalid url",
		"notification_id": w.raw.NotificationID,
	}))

	return err == nil
}

func (w *evergreenWebhookMessage) Raw() interface{} {
	return &w.raw
}

func (w *evergreenWebhookMessage) String() string {
	return string(w.raw.Body)
}

func (w *EvergreenWebhook) request() (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, w.URL, bytes.NewReader(w.Body))
	if err != nil {
		return nil, errors.Wrap(err, "creating webhook HTTP request")
	}

	hash, err := CalculateHMACHash(w.Secret, w.Body)
	if err != nil {
		return nil, errors.Wrap(err, "calculating HMAC hash")
	}

	for k := range w.Headers {
		for i := range w.Headers[k] {
			req.Header.Add(k, w.Headers[k][i])
		}
	}

	// Deduplicate the evergreen headers.
	req.Header.Del(evergreenHMACHeader)
	req.Header.Del(evergreenNotificationIDHeader)

	req.Header.Add(evergreenHMACHeader, hash)
	req.Header.Add(evergreenNotificationIDHeader, w.NotificationID)

	return req, nil
}

type evergreenWebhookLogger struct {
	client *http.Client
	*send.Base
}

func NewEvergreenWebhookLogger() (send.Sender, error) {
	s := &evergreenWebhookLogger{
		Base: send.NewBase("evergreen"),
	}

	return s, nil
}

func (w *evergreenWebhookLogger) Send(m message.Composer) {
	if w.Level().ShouldLog(m) {
		if err := w.send(m); err != nil {
			w.ErrorHandler()(err, m)
		}
	}
}

func (w *evergreenWebhookLogger) send(m message.Composer) error {
	raw, ok := m.Raw().(*EvergreenWebhook)
	if !ok {
		return errors.Errorf("received unexpected composer %T", m.Raw())
	}
	timeout := defaultWebhookTimeout
	if raw.TimeoutMS > 0 {
		timeout = time.Duration(raw.TimeoutMS) * time.Millisecond
	}
	minDelay := defaultMinDelay
	if raw.MinDelayMS > 0 {
		minDelay = time.Duration(raw.MinDelayMS) * time.Millisecond
	}

	client := w.client
	return utility.Retry(context.Background(), func() (bool, error) {
		req, err := raw.request()
		if err != nil {
			return false, errors.Wrap(err, "making webhook request")
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		req = req.WithContext(ctx)

		if client == nil {
			client = utility.GetHTTPClient()
			defer utility.PutHTTPClient(client)
		}

		resp, err := client.Do(req)
		if resp != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			return true, errors.Wrap(err, "sending webhook data")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return true, errors.Errorf("response was %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode))
		}

		return false, nil
	}, utility.RetryOptions{
		MaxAttempts: raw.Retries + 1,
		MinDelay:    minDelay,
	})
}

func (w *evergreenWebhookLogger) Flush(_ context.Context) error { return nil }
