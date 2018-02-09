package model

import (
	"errors"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	Collection = "github_hooks"
)

type GithubHook struct {
	HookID int    `bson:"_id"`
	Owner  string `bson:"owner"`
	Repo   string `bson:"repo"`
}

var (
	hookIDKey = bsonutil.MustHaveTag(GithubHook{}, "HookID") //nolint: deadcode, megacheck

	ownerKey = bsonutil.MustHaveTag(GithubHook{}, "Owner")
	repoKey  = bsonutil.MustHaveTag(GithubHook{}, "Repo")
)

func (h *GithubHook) Insert() error {
	if h.HookID == 0 {
		return errors.New("GithubHook ID must not be 0")
	}
	if len(h.Owner) == 0 || len(h.Repo) == 0 {
		return errors.New("Owner and repository must not be empty strings")
	}

	return db.Insert(Collection, h)
}

func FindGithubHook(owner, repo string) (*GithubHook, error) {
	if len(owner) == 0 || len(repo) == 0 {
		return nil, errors.New("Owner and repository must not be empty strings")
	}

	hook := &GithubHook{}
	err := db.FindOneQ(Collection, db.Query(bson.M{
		ownerKey: owner,
		repoKey:  repo,
	}), hook)

	if err == mgo.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return hook, nil
}
