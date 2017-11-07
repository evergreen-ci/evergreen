package db

import (
	"crypto/tls"
	"net"
	"sync"
	"time"

	"gopkg.in/mgo.v2"
)

var (
	globalSessionProvider SessionProvider = nil
	defaultSocketTimeout                  = 90 * time.Second
	mu                    sync.RWMutex
)

// SessionFactory contains information for connecting to Evergreen's
// MongoDB instance. Implements SessionProvider.
type SessionFactory struct {
	url           string
	db            string
	ssl           bool
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

// NewSessionFactory returns a new session factory pointed at the given URL/DB combo,
// with the supplied timeout and writeconcern settings.
func NewSessionFactory(url, db string, ssl bool, safety mgo.Safe, dialTimeout time.Duration) *SessionFactory {
	return &SessionFactory{
		url:           url,
		db:            db,
		ssl:           ssl,
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
			dialInfo, err := mgo.ParseURL(sf.url)
			if err != nil {
				return nil, nil, err
			}
			dialInfo.Timeout = sf.dialTimeout

			if sf.ssl {
				tlsConfig := &tls.Config{}
				// Note: this turns off certificate validation. TODO: load system certs and/or
				// allow the user to specify their own CA certs file
				tlsConfig.InsecureSkipVerify = true
				dialInfo.DialServer = func(addr *mgo.ServerAddr) (net.Conn, error) {
					conn, err := tls.Dial("tcp", addr.String(), tlsConfig)
					return conn, err
				}
			}

			sf.masterSession, err = mgo.DialWithInfo(dialInfo)
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
	mu.Lock()
	defer mu.Unlock()
	globalSessionProvider = sessionProvider
}

// GetGlobalSessionFactory returns the global session provider.
func GetGlobalSessionFactory() SessionProvider {
	mu.RLock()
	defer mu.RUnlock()
	if globalSessionProvider == nil {
		panic("No global session provider has been set.")
	}
	return globalSessionProvider
}

func HasGlobalSessionProvider() bool { return globalSessionProvider != nil }
