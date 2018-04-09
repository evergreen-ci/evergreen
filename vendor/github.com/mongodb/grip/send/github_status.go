package send

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/google/go-github/github"
	"github.com/mongodb/grip/message"
)

type githubStatusLogger struct {
	opts *GithubOptions
	ref  string

	gh githubClient
	*Base
}

func (s *githubStatusLogger) Send(m message.Composer) {
	if s.Level().ShouldLog(m) {
		var status *github.RepoStatus

		switch v := m.Raw().(type) {
		case *github.RepoStatus:
			status = v
		case *message.Fields:
			status = githubMessageFieldsToStatus(v)
		case message.Fields:
			status = githubMessageFieldsToStatus(&v)
		}
		if status == nil {
			s.ErrorHandler(errors.New("composer cannot be converted to github status"), m)
			return
		}

		_, _, err := s.gh.CreateStatus(context.TODO(), s.opts.Account, s.opts.Repo, s.ref, status)
		if err != nil {
			s.ErrorHandler(err, m)
		}
	}
}

// NewGithubStatusLogger returns a Sender to send payloads to the Github Status
// API. Statuses will be attached to the given ref.
func NewGithubStatusLogger(name string, opts *GithubOptions, ref string) (Sender, error) {
	s := &githubStatusLogger{
		Base: NewBase(name),
		gh:   &githubClientImpl{},
		ref:  ref,
	}

	ctx := context.TODO()
	s.gh.Init(ctx, opts.Token)

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := s.SetErrorHandler(ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	if err := s.SetFormatter(MakePlainFormatter()); err != nil {
		return nil, err
	}

	s.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s] [%s/%s] ", s.Name(), opts.Account, opts.Repo))
	}

	return s, nil
}

func githubMessageFieldsToStatus(m *message.Fields) *github.RepoStatus {
	if m == nil {
		return nil
	}

	state, ok := getStringPtrFromField((*m)["state"])
	if !ok {
		return nil
	}
	context, ok := getStringPtrFromField((*m)["context"])
	if !ok {
		return nil
	}
	URL, ok := getStringPtrFromField((*m)["URL"])
	if !ok {
		return nil
	}
	var description *string
	if description != nil {
		description, ok = getStringPtrFromField((*m)["description"])
		if description != nil && len(*description) == 0 {
			description = nil
		}
		if !ok {
			return nil
		}
	}

	return &github.RepoStatus{
		State:       state,
		Context:     context,
		URL:         URL,
		Description: description,
	}
}

func getStringPtrFromField(i interface{}) (*string, bool) {
	if ret, ok := i.(string); ok {
		return &ret, true
	}
	if ret, ok := i.(*string); ok {
		return ret, ok
	}

	return nil, false
}
