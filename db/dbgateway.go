package db

import (
	"github.com/evergreen-ci/evergreen"
	"labix.org/v2/mgo"
	"sync"
	"time"
)

var (
	globalSessionProvider SessionProvider = nil
)

type SessionFactory struct {
	Url           string
	DBName        string
	DialTimeout   time.Duration
	dialLock      sync.Mutex
	masterSession *mgo.Session
}

type SessionProvider interface {
	GetSession() (*mgo.Session, *mgo.Database, error)
}

func SessionFactoryFromConfig(appConf *evergreen.MCISettings) *SessionFactory {
	return NewSessionFactory(appConf.DbUrl, appConf.Db, 5*time.Second)
}

func NewSessionFactory(Url string, DBName string, Timeout time.Duration) *SessionFactory {
	return &SessionFactory{
		Url:         Url,
		DBName:      DBName,
		DialTimeout: Timeout,
	}
}

func (self *SessionFactory) GetSession() (*mgo.Session, *mgo.Database, error) {
	// if the master session has not been initialized, do that for the first time
	if self.masterSession == nil {
		self.dialLock.Lock()
		defer self.dialLock.Unlock()
		if self.masterSession == nil { //check again in case someone else just set and unlocked it
			var err error
			self.masterSession, err = mgo.DialWithTimeout(self.Url, self.DialTimeout)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	// copy the master session
	sessionCopy := self.masterSession.Copy()
	return sessionCopy, sessionCopy.DB(self.DBName), nil
}

func SetGlobalSessionProvider(sessionProvider SessionProvider) {
	globalSessionProvider = sessionProvider
}

func GetGlobalSessionFactory() SessionProvider {
	if globalSessionProvider == nil {
		panic("No global session provider has been set.")
	}
	return globalSessionProvider
}
