package send

import (
	"fmt"
	"log"
	"os"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

type jiraCommentJournal struct {
	issueID string
	opts    *JiraOptions
	*Base
}

// MakeJiraCommentLogger is the same as NewJiraCommentLogger but uses a warning
// level of Trace
func MakeJiraCommentLogger(id string, opts *JiraOptions) (Sender, error) {
	return NewJiraCommentLogger(id, opts, LevelInfo{level.Trace, level.Trace})
}

// NewJiraCommentLogger constructs a Sender that creates issues to jira, given
// options defined in a JiraOptions struct. id parameter is the ID of the issue
func NewJiraCommentLogger(id string, opts *JiraOptions, l LevelInfo) (Sender, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	j := &jiraCommentJournal{
		opts:    opts,
		issueID: id,
		Base:    NewBase(id),
	}

	if err := j.opts.client.CreateClient(opts.HTTPClient, opts.BaseURL); err != nil {
		return nil, err
	}

	if err := j.opts.client.Authenticate(opts.Username, opts.Password); err != nil {
		return nil, fmt.Errorf("jira authentication error: %v", err)
	}

	if err := j.SetLevel(l); err != nil {
		return nil, err
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := j.SetErrorHandler(ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	j.SetName(id)
	j.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s] ", j.Name()))
	}

	return j, nil
}

// Send post issues via jiraCommentJournal with information in the message.Composer
func (j *jiraCommentJournal) Send(m message.Composer) {
	if j.Level().ShouldLog(m) {
		if err := j.opts.client.PostComment(j.issueID, m.String()); err != nil {
			j.ErrorHandler(err, m)
		}
	}
}
