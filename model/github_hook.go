package model

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/github"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	GithubHooksCollection = "github_hooks"
)

type GithubHook struct {
	HookID int    `bson:"hook_id"`
	Owner  string `bson:"owner"`
	Repo   string `bson:"repo"`
}

//nolint: deadcode, megacheck, unused
var (
	hookIDKey = bsonutil.MustHaveTag(GithubHook{}, "HookID")
	ownerKey  = bsonutil.MustHaveTag(GithubHook{}, "Owner")
	repoKey   = bsonutil.MustHaveTag(GithubHook{}, "Repo")

	githubHookURL = fmt.Sprintf("%s/rest/v2/hooks/github", settings.ApiUrl)
)

func (h *GithubHook) Insert() error {
	if h.HookID == 0 {
		return errors.New("GithubHook ID must not be 0")
	}
	if len(h.Owner) == 0 || len(h.Repo) == 0 {
		return errors.New("Owner and repository must not be empty strings")
	}

	return db.Insert(GithubHooksCollection, h)
}

func FindGithubHook(owner, repo string) (*GithubHook, error) {
	if len(owner) == 0 || len(repo) == 0 {
		return nil, errors.New("Owner and repository must not be empty strings")
	}

	hook := &GithubHook{}
	err := db.FindOneQ(GithubHooksCollection, db.Query(bson.M{
		ownerKey: owner,
		repoKey:  repo,
	}), hook)

	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return hook, nil
}

func SetupNewGithubHook(ctx context.Context, settings evergreen.Settings, owner string, repo string) (*GithubHook, error) {
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, err
	}
	if settings.Api.GithubWebhookSecret == "" {
		return nil, errors.New("Evergreen is not configured for Github Webhooks")
	}

	httpClient, err := util.GetOAuth2HTTPClient(token)
	if err != nil {
		return nil, err
	}
	defer util.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)
	hookObj := github.Hook{
		Name:   github.String("web"),
		Active: github.Bool(true),
		Events: []string{"*"},
		Config: map[string]interface{}{
			"url":          github.String(githubHookURL),
			"content_type": github.String("json"),
			"secret":       github.String(settings.Api.GithubWebhookSecret),
			"insecure_ssl": github.String("0"),
		},
	}
	newCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp := &github.Response{}
	respHook := &github.Hook{}
	respHook, resp, err = client.Repositories.CreateHook(newCtx, owner, repo, &hookObj)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || respHook == nil || respHook.ID == nil {
		return nil, errors.New("unexpected data from github")
	}
	hook := &GithubHook{
		HookID: *respHook.ID,
		Owner:  owner,
		Repo:   repo,
	}
	return hook, nil
}

func GetExistingGithubHooks(ctx context.Context, settings evergreen.Settings, owner, repo string) (*GithubHook, error) {
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, err
	}
	httpClient, err := util.GetOAuth2HTTPClient(token)
	if err != nil {
		return nil, err
	}
	defer util.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	respHooks, resp, err := client.Repositories.ListHooks(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get hooks for owner '%s', repo '%s'", owner, repo)
	}

	for _, hook := range respHooks {
		if hook.GetURL() == githubHookURL {
			return &GithubHook{
				HookID: *hook.ID,
				Owner:  owner,
				Repo:   repo,
			}, nil
		}
	}

	return nil, errors.Errorf("no matching hooks found")
}
