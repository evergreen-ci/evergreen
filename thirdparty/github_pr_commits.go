package thirdparty

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var CoAuthorPattern = regexp.MustCompile(`(?i)Co-[Aa]uthored-[Bb]y:\s*(.+?)\s*<(.+?)>`)

func GetGithubPRCommits(ctx context.Context, owner, repo string, prNumber int) ([]*github.RepositoryCommit, *github.Response, error) {
	caller := "GetGithubPRCommits"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	commits, resp, err := githubClient.PullRequests.ListCommits(ctx, owner, repo, prNumber, nil)
	if resp != nil {
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
		if err != nil {
			return nil, resp, parseGithubErrorResponse(resp)
		}
	} else if err != nil {
		errMsg := fmt.Sprintf("nil response from query for commits in PR '%s/%s' #%d : %v", owner, repo, prNumber, err)
		grip.Error(errMsg)
		return nil, nil, APIResponseError{errMsg}
	}

	return commits, resp, nil
}

func ExtractCoAuthorFromCommit(commit *github.RepositoryCommit) (coAuthorName, coAuthorEmail string) {
	if commit == nil || commit.Commit == nil || commit.Commit.Message == nil {
		return "", ""
	}

	message := *commit.Commit.Message
	matches := CoAuthorPattern.FindStringSubmatch(message)
	if len(matches) >= 3 {
		return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2])
	}
	return "", ""
}

func GetGitHubUsernameFromEmail(commit *github.RepositoryCommit, email string) string {
	if commit != nil {
		if commit.Commit != nil && commit.Commit.Author != nil &&
			commit.Commit.Author.Email != nil && *commit.Commit.Author.Email == email {
			if commit.Author != nil && commit.Author.Login != nil {
				return *commit.Author.Login
			}
		}

		if commit.Commit != nil && commit.Commit.Committer != nil &&
			commit.Commit.Committer.Email != nil && *commit.Commit.Committer.Email == email {
			if commit.Committer != nil && commit.Committer.Login != nil {
				return *commit.Committer.Login
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, err := GetGithubUserByEmail(ctx, email)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "Failed to look up GitHub user by email",
			"email":   email,
			"ticket":  "DEVPROD-16345",
		}))
		return ""
	}

	if user != nil && user.Login != nil {
		return *user.Login
	}

	return ""
}
