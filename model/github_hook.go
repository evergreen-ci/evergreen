package model

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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

//nolint:megacheck,unused
var (
	hookIDKey = bsonutil.MustHaveTag(GithubHook{}, "HookID")
	ownerKey  = bsonutil.MustHaveTag(GithubHook{}, "Owner")
	repoKey   = bsonutil.MustHaveTag(GithubHook{}, "Repo")
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
	githubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	respHook, err := thirdparty.CreateGithubHook(githubCtx, settings, owner, repo)
	if err != nil {
		return nil, err
	}
	grip.Debug(message.Fields{
		"ticket":            "EVG-15779",
		"setup new webhook": respHook,
		"owner":             owner,
		"repo":              repo,
	})
	hook := &GithubHook{
		HookID: int(respHook.GetID()),
		Owner:  owner,
		Repo:   repo,
	}
	return hook, nil
}

func GetExistingGithubHook(ctx context.Context, settings evergreen.Settings, owner, repo string) (*GithubHook, error) {
	githubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	hook, err := thirdparty.GetExistingGithubHook(githubCtx, settings, owner, repo)
	if err != nil {
		return nil, err
	}
	return &GithubHook{
		HookID: int(hook.GetID()),
		Owner:  owner,
		Repo:   repo,
	}, nil
}

func RemoveGithubHook(hookID int) error {
	hook, err := FindGithubHookByID(hookID)
	if err != nil {
		return errors.Wrap(err, "finding hooks")
	}
	if hook == nil {
		return errors.Errorf("no hook found for ID '%d'", hookID)
	}
	return errors.Wrapf(hook.Remove(), "removing hook with ID '%d'", hookID)
}
