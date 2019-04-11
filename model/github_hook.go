package model

import (
	"errors"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
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
