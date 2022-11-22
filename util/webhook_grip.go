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
	evergreenWebhookTimeout       = 30 * time.Second
	evergreenNotificationIDHeader = "X-Evergreen-Notification-ID"
	evergreenHMACHeader           = "X-Evergreen-Signature"
)

type EvergreenWebhook struct {
	NotificationID string        `bson:"notification_id"`
	URL            string        `bson:"url"`
	Secret         []byte        `bson:"secret"`
	Body           []byte        `bson:"body"`
	Headers        http.Header   `bson:"headers"`
	Retries        int           `bson:"retries"`
	MinDelay       time.Duration `bson:"min_delay"`
}

type evergreenWebhookMessage struct {
	raw EvergreenWebhook

	message.Base
}

func NewWebhookMessageWithStruct(raw EvergreenWebhook) message.Composer {
	return &evergreenWebhookMessage{
		raw: raw,
	}
}

func NewWebhookMessage(id string, url string, secret []byte, body []byte, headers map[string][]string) message.Composer {
	return &evergreenWebhookMessage{
		raw: EvergreenWebhook{
			NotificationID: id,
			URL:            url,
			Secret:         secret,
			Body:           body,
			Headers:        headers,
		},
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

	reader := bytes.NewReader(raw.Body)
	req, err := http.NewRequest(http.MethodPost, raw.URL, reader)
	if err != nil {
		return errors.Wrap(err, "creating webhook HTTP request")
	}

	hash, err := CalculateHMACHash(raw.Secret, raw.Body)
	if err != nil {
		return errors.Wrap(err, "calculating HMAC hash")
	}

	for k := range raw.Headers {
		for i := range raw.Headers[k] {
			req.Header.Add(k, raw.Headers[k][i])
		}
	}

	req.Header.Del(evergreenHMACHeader)
	req.Header.Add(evergreenHMACHeader, hash)
	req.Header.Del(evergreenNotificationIDHeader)
	req.Header.Add(evergreenNotificationIDHeader, raw.NotificationID)

	ctx, cancel := context.WithTimeout(req.Context(), evergreenWebhookTimeout)
	defer cancel()

	req = req.WithContext(ctx)

	var client *http.Client = w.client
	utility.Retry(ctx, func() (bool, error) {
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
			return false, errors.Errorf("response was %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode))
		}

		return false, nil
	}, utility.RetryOptions{
		MaxAttempts: raw.Retries + 1,
		MinDelay:    raw.MinDelay,
	})

	return nil
}

func (w *evergreenWebhookLogger) Flush(_ context.Context) error { return nil }
