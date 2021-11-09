package remote

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

// restLoggingCache is the client-side representation of a jasper.LoggingCache
// for making requests to the remote REST service.
type restLoggingCache struct {
	client *restClient
	ctx    context.Context
}

func (lc *restLoggingCache) Create(id string, opts *options.Output) (*options.CachedLogger, error) {
	body, err := makeBody(opts)
	if err != nil {
		return nil, errors.Wrap(err, "building request")
	}

	resp, err := lc.client.doRequest(lc.ctx, http.MethodPost, lc.client.getURL("/logging/id/%s", id), body)
	if err != nil {
		return nil, errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	if err = handleError(resp); err != nil {
		return nil, errors.WithStack(err)
	}

	out := &options.CachedLogger{}
	if err = gimlet.GetJSON(resp.Body, out); err != nil {
		return nil, errors.Wrap(err, "getting cached logger info from response")
	}

	return out, nil
}

func (lc *restLoggingCache) Put(id string, cl *options.CachedLogger) error {
	return errors.New("operation not supported for remote managers")
}

func (lc *restLoggingCache) Get(id string) (*options.CachedLogger, error) {
	resp, err := lc.client.doRequest(lc.ctx, http.MethodGet, lc.client.getURL("/logging/id/%s", id), nil)
	if err != nil {
		return nil, errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	if err = handleError(resp); err != nil {
		return nil, errors.WithStack(err)
	}

	out := &options.CachedLogger{}
	if err = gimlet.GetJSON(resp.Body, out); err != nil {
		return nil, errors.Wrap(err, "getting cached logger info from response")
	}
	return out, nil
}

func (lc *restLoggingCache) Remove(id string) error {
	resp, err := lc.client.doRequest(lc.ctx, http.MethodDelete, lc.client.getURL("/logging/id/%s", id), nil)
	if err != nil {
		return errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	return errors.WithStack(handleError(resp))
}

func (lc *restLoggingCache) CloseAndRemove(ctx context.Context, id string) error {
	resp, err := lc.client.doRequest(ctx, http.MethodDelete, lc.client.getURL("/logging/id/%s/close", id), nil)
	if err != nil {
		return errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	return errors.WithStack(handleError(resp))
}

func (lc *restLoggingCache) Clear(ctx context.Context) error {
	resp, err := lc.client.doRequest(ctx, http.MethodDelete, lc.client.getURL("/logging/clear"), nil)
	if err != nil {
		return errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	return errors.WithStack(handleError(resp))
}

func (lc *restLoggingCache) Prune(ts time.Time) error {
	resp, err := lc.client.doRequest(lc.ctx, http.MethodDelete, lc.client.getURL("/logging/prune/%s", ts.Format(time.RFC3339)), nil)
	if err != nil {
		return errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	return errors.WithStack(handleError(resp))
}

func (lc *restLoggingCache) Len() (int, error) {
	resp, err := lc.client.doRequest(lc.ctx, http.MethodGet, lc.client.getURL("/logging/len"), nil)
	if err != nil {
		return -1, errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	if err := handleError(resp); err != nil {
		return -1, errors.WithStack(err)
	}

	out := restLoggingCacheLen{}
	if err = gimlet.GetJSON(resp.Body, &out); err != nil {
		return -1, errors.Wrap(err, "getting logging cache length from response")
	}

	return out.Len, nil
}
