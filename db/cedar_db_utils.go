package db

import (
	"context"
	"errors"
	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/anser/db"
)

type cedarShimFactoryImpl struct {
	env evergreen.Environment
	db  string
}

func GetCedarGlobalSessionFactory() SessionFactory {
	env := evergreen.GetEnvironment()
	return &cedarShimFactoryImpl{
		env: env,
		db:  env.Settings().Cedar.DbName,
	}
}

// GetCedarContextSession creates a cedar database session and connection that uses the associated
// context in its operations.
func (s *cedarShimFactoryImpl) GetContextSession(ctx context.Context) (db.Session, db.Database, error) {
	if s.env == nil {
		return nil, nil, errors.New("undefined environment")
	}

	session := s.env.CedarContextSession(ctx)
	if session == nil {
		return nil, nil, errors.New("context session is not defined")
	}

	return session, session.DB(s.db), nil
}

// GetSession creates a database connection using the global environment's
// session (and context through the session).
func (s *cedarShimFactoryImpl) GetSession() (db.Session, db.Database, error) {
	if s.env == nil {
		return nil, nil, errors.New("undefined environment")
	}

	session := s.env.Session()
	if session == nil {
		return nil, nil, errors.New("session is not defined")
	}

	return session, session.DB(s.db), nil
}

// FindOneQContextCedar runs a Q query against the given collection, applying the results to "out."
// Only reads one document from the DB.
func FindOneQContextCedar(ctx context.Context, collection string, q Q, out any) error {
	if q.maxTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, q.maxTime)
		defer cancel()
	}

	session, db, err := GetCedarGlobalSessionFactory().GetContextSession(ctx)
	if err != nil {
		return err
	}
	defer session.Close()

	return db.C(collection).
		Find(q.filter).
		Select(q.projection).
		Sort(q.sort...).
		Skip(q.skip).
		Limit(1).
		Hint(q.hint).
		One(out)
}
