package commitqueue

import (
	"context"

	"github.com/google/go-github/github"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

type mockGithubPRLogger struct {
	githubPRLogger
	errLogger *mockErrorLogger
}

// Mock implementation
func NewMockGithubPRLogger(name string, errLogger *mockErrorLogger) (send.Sender, error) {
	s := &mockGithubPRLogger{
		githubPRLogger: githubPRLogger{
			Base:         send.NewBase(name),
			prService:    &mockPRService{},
			statusSender: mockStatusSender{},
			ctx:          context.Background(),
		},
		errLogger: errLogger,
	}

	if err := s.SetErrorHandler(errLogger.logError); err != nil {
		return nil, errors.Wrap(err, "can't set mock github pr logger error handler")
	}

	return s, nil
}

type mockPRService struct{}

func (mockPRService) Merge(ctx context.Context, owner string, repo string, number int, commitMessage string, options *github.PullRequestOptions) (*github.PullRequestMergeResult, *github.Response, error) {
	trueVar := true
	return &github.PullRequestMergeResult{Merged: &trueVar}, nil, nil
}

type mockStatusSender struct{}

func (mockStatusSender) Send(c message.Composer) {}

type mockErrorLogger struct {
	errList []error
}

func (e *mockErrorLogger) logError(err error, m message.Composer) {
	if err == nil {
		return
	}

	e.errList = append(e.errList, err)
}
