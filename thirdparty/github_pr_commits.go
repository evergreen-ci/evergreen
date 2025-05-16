package thirdparty

import (
	"context"
	"fmt"

	"github.com/google/go-github/v70/github"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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
