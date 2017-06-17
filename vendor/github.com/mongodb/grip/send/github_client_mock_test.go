package send

import (
	"github.com/google/go-github/github"
	"golang.org/x/net/context"
)

type githubClientMock struct {
	failCreate bool
	failSend   bool
	numSent    int
}

func (g *githubClientMock) Init(_ context.Context, _ string) {}

func (g *githubClientMock) Create(_ context.Context, _ string, _ string, _ *github.IssueRequest) (*github.Issue, *github.Response, error) {
	return nil, nil, nil
}
func (g *githubClientMock) CreateComment(_ context.Context, _ string, _ string, _ int, _ *github.IssueComment) (*github.IssueComment, *github.Response, error) {
	return nil, nil, nil
}
