package util

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

const (
	defaultWebhookTimeout         = 30 * time.Second
	defaultMinDelay               = 500 * time.Millisecond
	maxWebhookResponseDrainSize   = 64 * 1024
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

	err := ValidateWebhookURL(w.raw.URL)
	if err != nil {
		grip.Error(context.Background(), message.WrapError(err, message.Fields{
			"message":         "evergreen-webhook invalid url",
			"notification_id": w.raw.NotificationID,
		}))
	}

	return err == nil
}

func (w *evergreenWebhookMessage) Raw() any {
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
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A proxy could reach an internal destination without using the guarded direct dialer.
	transport.Proxy = nil
	// Webhook destinations are user-controlled, so validate the resolved address for every connection.
	transport.DialContext = webhookDialContext(net.DefaultResolver)

	s := &evergreenWebhookLogger{
		client: utility.WithOTelTracing(&http.Client{
			Transport:     transport,
			CheckRedirect: validateWebhookRedirect,
		}),
		Base: send.NewBase("evergreen"),
	}

	return s, nil
}

func (w *evergreenWebhookLogger) Send(ctx context.Context, m message.Composer) {
	if w.Level().ShouldLog(m) {
		if err := w.send(m); err != nil {
			w.ErrorHandler()(ctx, err, m)
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

		resp, err := client.Do(req)
		msgFields := message.Fields{
			"message":         "error sending webhook notification",
			"notification_id": raw.NotificationID,
			"webhook_url":     raw.URL,
			"is_ctx_err":      utility.IsContextError(ctx.Err()),
		}
		if err != nil {
			return true, message.WrapError(errors.Wrap(err, "sending webhook data"), msgFields)
		}

		defer resp.Body.Close()

		msgFields["status_code"] = resp.StatusCode

		// Endpoint response bodies may contain sensitive data, so do not retain them in operator logs.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxWebhookResponseDrainSize))

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return true, message.WrapError(errors.Errorf("webhook response was %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode)), msgFields)
		}

		msgFields["message"] = "successfully sent webhook notification"
		grip.Info(ctx, msgFields)

		return false, nil
	}, utility.RetryOptions{
		MaxAttempts: raw.Retries + 1,
		MinDelay:    minDelay,
	})
}

func (w *evergreenWebhookLogger) Flush(_ context.Context) error { return nil }

// ValidateWebhookURL rejects destination forms that could turn Evergreen into a proxy for local services.
// Hostname resolution is repeated immediately before dialing to protect against DNS rebinding.
func ValidateWebhookURL(raw string) error {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return errors.Wrap(err, "parsing webhook URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("webhook URL must use HTTP or HTTPS")
	}
	if u.Hostname() == "" {
		return errors.New("webhook URL must have a host")
	}
	if u.User != nil {
		return errors.New("webhook URL cannot contain user info")
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && isBlockedWebhookIP(ip) {
		return errors.Errorf("webhook URL cannot use blocked address %s", ip)
	}

	return nil
}

type webhookResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

// webhookDialContext prevents hostname changes from redirecting requests to local services.
func webhookDialContext(resolver webhookResolver) func(context.Context, string, string) (net.Conn, error) {
	dialer := net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.Wrap(err, "splitting webhook destination address")
		}

		// Hostname records can change after validation, so resolve them immediately before connecting.
		ips, err := resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, errors.Wrapf(err, "resolving webhook destination %s", host)
		}
		if len(ips) == 0 {
			return nil, errors.Errorf("webhook destination %s did not resolve", host)
		}
		for _, ip := range ips {
			if isBlockedWebhookIP(ip) {
				return nil, errors.Errorf("webhook destination %s resolves to blocked address %s", host, ip)
			}
		}

		var lastErr error
		for _, ip := range ips {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}

		return nil, errors.Wrapf(lastErr, "dialing webhook destination %s", host)
	}
}

// validateWebhookRedirect prevents redirects from bypassing destination validation.
func validateWebhookRedirect(req *http.Request, _ []*http.Request) error {
	return ValidateWebhookURL(req.URL.String())
}

// isBlockedWebhookIP keeps validation and dialing on the same internal-address policy.
func isBlockedWebhookIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return !ip.IsValid() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsMulticast()
}
