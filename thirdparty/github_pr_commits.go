package thirdparty

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-github/v70/github"
	"github.com/mongodb/grip"
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
