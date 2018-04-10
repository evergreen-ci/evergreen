package notification

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

const (
	evergreenWebhookTimeout       = 10 * time.Second
	evergreenNotificationIDHeader = "X-Evergreen-Notification-ID"
	evergreenHMACHeader           = "X-Evergreen-Signature"
)

type evergreenWebhookMessage struct {
	id     string
	url    *url.URL
	secret []byte
	body   []byte

	message.Base
}

func NewWebhookMessage(id string, url *url.URL, secret []byte, body []byte) message.Composer {
	return &evergreenWebhookMessage{
		id:     id,
		url:    url,
		secret: secret,
		body:   body,
	}
}

func (w *evergreenWebhookMessage) Loggable() bool {
	return w.url != nil && len(w.id) != 0 && len(w.secret) != 0 && len(w.body) != 0
}

func (w *evergreenWebhookMessage) Raw() interface{} {
	return w
}

func (w *evergreenWebhookMessage) String() string {
	return string(w.body)
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
			w.ErrorHandler(err, m)
		}
	}
}

func (w *evergreenWebhookLogger) send(m message.Composer) error {
	raw, ok := m.Raw().(*evergreenWebhookMessage)
	if !ok {
		return errors.New("evergreen-webhook unexpected composer")
	}

	reader := bytes.NewReader(raw.body)
	req, err := http.NewRequest(http.MethodPost, raw.url.String(), reader)
	if err != nil {
		return errors.Wrap(err, "evergreen-webhook failed to create http request")
	}

	hash, err := util.CalculateHMACHash(raw.secret, raw.body)
	if err != nil {
		return errors.Wrap(err, "evergreen-webhook failed to calculate hash")
	}

	req.Header.Del(evergreenHMACHeader)
	req.Header.Add(evergreenHMACHeader, hash)
	req.Header.Del(evergreenNotificationIDHeader)
	req.Header.Add(evergreenNotificationIDHeader, raw.id)

	ctx, cancel := context.WithTimeout(req.Context(), evergreenWebhookTimeout)
	defer cancel()

	req = req.WithContext(ctx)

	var client *http.Client = w.client
	if client == nil {
		client = util.GetHTTPClient()
		defer util.PutHTTPClient(client)
	}

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return errors.Wrap(err, "evergreen-webhook failed to send webhook data")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.Errorf("evergreen-webhook response status was %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	return nil
}
