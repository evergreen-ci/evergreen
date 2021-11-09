package remote

import (
	"bytes"
	"context"
	"net/http"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/jasper/scripting"
	"github.com/pkg/errors"
)

// restScriptingHarness is the client-side representation of a
// scripting.Harness for making requests to the remote REST service.
type restScriptingHarness struct {
	id     string
	client *restClient
}

func newRESTScriptingHarness(client *restClient, id string) *restScriptingHarness {
	return &restScriptingHarness{
		id:     id,
		client: client,
	}
}

func (s *restScriptingHarness) ID() string { return s.id }
func (s *restScriptingHarness) Setup(ctx context.Context) error {
	resp, err := s.client.doRequest(ctx, http.MethodPost, s.client.getURL("/scripting/%s/setup", s.id), nil)
	if err != nil {
		return errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()
	return nil
}

func (s *restScriptingHarness) Run(ctx context.Context, args []string) error {
	body, err := makeBody(struct {
		Args []string `json:"args"`
	}{Args: args})
	if err != nil {
		return errors.Wrap(err, "problem building request")
	}

	resp, err := s.client.doRequest(ctx, http.MethodPost, s.client.getURL("/scripting/%s/run", s.id), body)
	if err != nil {
		return errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	return nil
}

func (s *restScriptingHarness) RunScript(ctx context.Context, script string) error {
	resp, err := s.client.doRequest(ctx, http.MethodPost, s.client.getURL("/scripting/%s/script", s.id), bytes.NewBuffer([]byte(script)))
	if err != nil {
		return errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	return nil
}

func (s *restScriptingHarness) Build(ctx context.Context, dir string, args []string) (string, error) {
	body, err := makeBody(struct {
		Directory string   `json:"directory"`
		Args      []string `json:"args"`
	}{
		Directory: dir,
		Args:      args,
	})
	if err != nil {
		return "", errors.Wrap(err, "problem building request")
	}

	resp, err := s.client.doRequest(ctx, http.MethodPost, s.client.getURL("/scripting/%s/build", s.id), body)
	if err != nil {
		return "", errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	out := struct {
		Path string `json:"path"`
	}{}

	if err = gimlet.GetJSON(resp.Body, &out); err != nil {
		return "", errors.Wrap(err, "problem reading response")
	}

	return out.Path, nil
}

func (s *restScriptingHarness) Test(ctx context.Context, dir string, args ...scripting.TestOptions) ([]scripting.TestResult, error) {
	body, err := makeBody(struct {
		Directory string                  `json:"directory"`
		Options   []scripting.TestOptions `json:"options"`
	}{
		Directory: dir,
		Options:   args,
	})

	if err != nil {
		return nil, errors.Wrap(err, "problem building request")
	}

	resp, err := s.client.doRequest(ctx, http.MethodPost, s.client.getURL("/scripting/%s/test", s.id), body)
	if err != nil {
		return nil, errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	out := struct {
		Results []scripting.TestResult `json:"results"`
		Error   string                 `json:"error"`
	}{}

	if err = gimlet.GetJSON(resp.Body, &out); err != nil {
		return nil, errors.Wrap(err, "problem reading response")
	}

	if out.Error != "" {
		err = errors.New(out.Error)
	}

	return out.Results, err
}

func (s *restScriptingHarness) Cleanup(ctx context.Context) error {
	resp, err := s.client.doRequest(ctx, http.MethodDelete, s.client.getURL("/scripting/%s", s.id), nil)
	if err != nil {
		return errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	return nil
}
