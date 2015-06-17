package db

import (
	"github.com/evergreen-ci/evergreen"
	"gopkg.in/mgo.v2"
	"sync"
	"time"
)

var (
	globalSessionProvider SessionProvider = nil
	defaultDialTimeout                    = 5 * time.Second
	defaultSocketTimeout                  = 90 * time.Second
)

// SessionFactory contains information for connecting to Evergreen's
// MongoDB instance. Implements SessionProvider.
type SessionFactory struct {
	url           string
	db            string
	dialTimeout   time.Duration
	socketTimeout time.Duration
	safety        mgo.Safe
	dialLock      sync.Mutex
	masterSession *mgo.Session
}

// SessionProvider returns mgo Sessions for database interaction.
type SessionProvider interface {
	GetSession() (*mgo.Session, *mgo.Database, error)
}

// SessionFactoryFromConfig creates a usable SessionFactory from
// the Evergreen settings.
func SessionFactoryFromConfig(settings *evergreen.Settings) *SessionFactory {
	safety := mgo.Safe{}
	safety.W = settings.WriteConcern.W
	safety.WMode = settings.WriteConcern.WMode
	safety.WTimeout = settings.WriteConcern.WTimeout
	safety.FSync = settings.WriteConcern.FSync
	safety.J = settings.WriteConcern.J
	return NewSessionFactory(settings.DbUrl, settings.Db, safety, defaultDialTimeout)
}

// NewSessionFactory returns a new session factory pointed at the given URL/DB combo,
// with the supplied timeout and writeconcern settings.
func NewSessionFactory(url, db string, safety mgo.Safe, dialTimeout time.Duration) *SessionFactory {
	return &SessionFactory{
		url:           url,
		db:            db,
		dialTimeout:   dialTimeout,
		socketTimeout: defaultSocketTimeout,
		safety:        safety,
	}
}

func (sf *SessionFactory) GetSession() (*mgo.Session, *mgo.Database, error) {
	// if the master session has not been initialized, do that for the first time
	if sf.masterSession == nil {
		sf.dialLock.Lock()
		defer sf.dialLock.Unlock()
		if sf.masterSession == nil { //check again in case someone else just set and unlocked it
			var err error
			sf.masterSession, err = mgo.DialWithTimeout(sf.url, sf.dialTimeout)
			if err != nil {
				return nil, nil, err
			}
			sf.masterSession.SetSocketTimeout(sf.socketTimeout)
			sf.masterSession.SetSafe(&sf.safety)
		}
	}

	// copy the master session
	sessionCopy := sf.masterSession.Copy()
	return sessionCopy, sessionCopy.DB(sf.db), nil
}

// SetGlobalSessionProvider sets the global session provider.
func SetGlobalSessionProvider(sessionProvider SessionProvider) {
	globalSessionProvider = sessionProvider
}

// GetGlobalSessionFactory returns the global session provider.
func GetGlobalSessionFactory() SessionProvider {
	if globalSessionProvider == nil {
		panic("No global session provider has been set.")
	}
	return globalSessionProvider
}
