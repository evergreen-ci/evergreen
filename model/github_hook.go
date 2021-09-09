package model

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
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

	githubHookURLString = "%s/rest/v2/hooks/github"
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

func (h *GithubHook) Remove() error {
	return errors.WithStack(db.Remove(GithubHooksCollection, bson.M{hookIDKey: h.HookID}))
}

func FindGithubHook(owner, repo string) (*GithubHook, error) {
	if len(owner) == 0 || len(repo) == 0 {
		return nil, nil
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

func FindGithubHookByID(hookID int) (*GithubHook, error) {
	hook := &GithubHook{}
	err := db.FindOneQ(GithubHooksCollection, db.Query(bson.M{
		hookIDKey: hookID,
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

	httpClient := utility.GetOAuth2HTTPClient(token)
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)
	hookObj := github.Hook{
		Active: github.Bool(true),
		Events: []string{"*"},
		Config: map[string]interface{}{
			"url":          github.String(fmt.Sprintf(githubHookURLString, settings.ApiUrl)),
			"content_type": github.String("json"),
			"secret":       github.String(settings.Api.GithubWebhookSecret),
			"insecure_ssl": github.String("0"),
		},
	}
	newCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	respHook, resp, err := client.Repositories.CreateHook(newCtx, owner, repo, &hookObj)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || respHook == nil || respHook.ID == nil {
		return nil, errors.New("unexpected data from github")
	}
	hook := &GithubHook{
		HookID: int(respHook.GetID()),
		Owner:  owner,
		Repo:   repo,
	}
	return hook, nil
}

func GetExistingGithubHook(ctx context.Context, settings evergreen.Settings, owner, repo string) (*GithubHook, error) {
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "can't get github token")
	}

	httpClient := utility.GetOAuth2HTTPClient(token)
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)
	newCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	respHooks, _, err := client.Repositories.ListHooks(newCtx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get hooks for owner '%s', repo '%s'", owner, repo)
	}

	url := fmt.Sprintf(githubHookURLString, settings.ApiUrl)
	for _, hook := range respHooks {
		if hook.Config["url"] == url {
			return &GithubHook{
				HookID: int(hook.GetID()),
				Owner:  owner,
				Repo:   repo,
			}, nil
		}
	}

	return nil, errors.Errorf("no matching hooks found")
}

func RemoveGithubHook(hookID int) error {
	hook, err := FindGithubHookByID(hookID)
	if err != nil {
		return errors.Wrap(err, "can't query for webhooks")
	}
	if hook == nil {
		return errors.Errorf("no hook found for id '%d'", hookID)
	}
	return errors.Wrapf(hook.Remove(), "can't remove hook with ID '%d'", hookID)
}
