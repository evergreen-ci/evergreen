package send

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/google/go-github/github"
	"github.com/mongodb/grip/message"
)

type githubCommentLogger struct {
	issue int
	opts  *GithubOptions
	gh    githubClient

	*Base
}

// NewGithubCommentLogger creates a new Sender implementation that
// adds a comment to a github issue (or pull request) for every log
// message sent.
//
// Specify the credentials to use the GitHub via the GithubOptions
// structure, and the issue number as an argument to the constructor.
func NewGithubCommentLogger(name string, issueID int, opts *GithubOptions) (Sender, error) {
	s := &githubCommentLogger{
		Base:  NewBase(name),
		opts:  opts,
		issue: issueID,
		gh:    &githubClientImpl{},
	}

	ctx := context.TODO()

	s.gh.Init(ctx, opts.Token)

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := s.SetErrorHandler(ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	if err := s.SetFormatter(MakeDefaultFormatter()); err != nil {
		return nil, err
	}

	s.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s] [%s/%s#%d] ",
			s.Name(), opts.Account, opts.Repo, issueID))
	}

	return s, nil
}

func (s *githubCommentLogger) Send(m message.Composer) {
	if s.Level().ShouldLog(m) {
		text, err := s.formatter(m)
		if err != nil {
			s.ErrorHandler(err, m)
			return
		}

		comment := &github.IssueComment{Body: &text}

		ctx := context.TODO()
		_, _, err = s.gh.CreateComment(ctx, s.opts.Account, s.opts.Repo, s.issue, comment)
		if err != nil {
			s.ErrorHandler(err, m)
		}
	}
}
