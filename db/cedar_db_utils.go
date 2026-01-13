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

// GetCedarGlobalSessionFactory initializes a session factory to connect to the Cedar database.
func GetCedarGlobalSessionFactory() SessionFactory {
	env := evergreen.GetEnvironment()
	return &cedarShimFactoryImpl{
		env: env,
		db:  env.Settings().Cedar.DBName,
	}
}

// GetSession creates a cedar database session and connection that uses the associated
// context in its operations.
func (s *cedarShimFactoryImpl) GetSession(ctx context.Context) (db.Session, db.Database, error) {
	if s.env == nil {
		return nil, nil, errors.New("undefined environment")
	}

	session := s.env.CedarSession(ctx)
	if session == nil {
		return nil, nil, errors.New("context session is not defined")
	}

	return session, session.DB(s.db), nil
}
