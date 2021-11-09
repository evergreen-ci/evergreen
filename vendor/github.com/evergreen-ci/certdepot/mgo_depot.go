package certdepot

import (
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/square/certstrap/depot"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type mgoCertDepot struct {
	session        *mgo.Session
	databaseName   string
	collectionName string
	opts           DepotOptions
}

// NewMgoCertDepot creates a new cert depot using the legacy mgo driver.
func NewMgoCertDepot(opts *MongoDBOptions) (Depot, error) {
	s, err := mgo.DialWithTimeout(opts.MongoDBURI, opts.MongoDBDialTimeout)
	if err != nil {
		return nil, errors.Wrapf(err, "could not connect to db %s", opts.MongoDBURI)
	}
	s.SetSocketTimeout(opts.MongoDBSocketTimeout)

	return NewMgoCertDepotWithSession(s, opts)
}

// NewMgoCertDepotWithSession creates a certificate depot using the provided
// legacy mgo drivers session.
func NewMgoCertDepotWithSession(s *mgo.Session, opts *MongoDBOptions) (Depot, error) {
	if s == nil {
		return nil, errors.New("must specify a non-nil session")
	}

	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid options!")
	}

	return &mgoCertDepot{
		session:        s,
		databaseName:   opts.DatabaseName,
		collectionName: opts.CollectionName,
		opts:           opts.DepotOptions,
	}, nil
}

// Put inserts the data into the document specified by the tag.
func (m *mgoCertDepot) Put(tag *depot.Tag, data []byte) error {
	if data == nil {
		return errors.New("data is nil")
	}

	name, key, err := getNameAndKey(tag)
	if err != nil {
		return errors.Wrapf(err, "could not format name %s", name)
	}
	session := m.session.Clone()
	defer session.Close()

	update := bson.M{"$set": bson.M{key: string(data)}}
	changeInfo, err := session.DB(m.databaseName).C(m.collectionName).UpsertId(name, update)
	if err != nil {
		return errors.Wrap(err, "problem adding data to the database")
	}
	grip.Debug(message.Fields{
		"db":     m.databaseName,
		"coll":   m.collectionName,
		"id":     name,
		"change": changeInfo,
		"op":     "put",
	})

	return nil
}

// Check returns whether the user and data specified by the tag exists.
func (m *mgoCertDepot) Check(tag *depot.Tag) bool {
	name, key, err := getNameAndKey(tag)
	if err != nil {
		return false
	}
	session := m.session.Clone()
	defer session.Close()

	u := &User{}
	err = session.DB(m.databaseName).C(m.collectionName).FindId(name).One(u)
	grip.WarningWhen(errNotNotFound(err), message.WrapError(err, message.Fields{
		"db":   m.databaseName,
		"coll": m.collectionName,
		"id":   name,
		"err":  err,
		"op":   "check",
	}))

	switch key {
	case userCertKey:
		return u.Cert != ""
	case userPrivateKeyKey:
		return u.PrivateKey != ""
	case userCertReqKey:
		return u.CertReq != ""
	case userCertRevocListKey:
		return u.CertRevocList != ""
	default:
		return false
	}
}

// CheckWithError returns whether the user and data specified by the tag exists
// as well as an error in the case of an internal error.
func (m *mgoCertDepot) CheckWithError(tag *depot.Tag) (bool, error) {
	name, key, err := getNameAndKey(tag)
	if err != nil {
		return false, errors.Wrap(err, "getting name and key")
	}
	session := m.session.Clone()
	defer session.Close()

	u := &User{}

	if err = session.DB(m.databaseName).C(m.collectionName).FindId(name).One(u); errNotNotFound(err) {
		return false, errors.Wrap(err, "checking depot tag")
	}

	switch key {
	case userCertKey:
		return u.Cert != "", nil
	case userPrivateKeyKey:
		return u.PrivateKey != "", nil
	case userCertReqKey:
		return u.CertReq != "", nil
	case userCertRevocListKey:
		return u.CertRevocList != "", nil
	default:
		return false, nil
	}
}

// Get reads the data for the user specified by tag. Returns an error if the
// user does not exist or if the data is empty.
func (m *mgoCertDepot) Get(tag *depot.Tag) ([]byte, error) {
	name, key, err := getNameAndKey(tag)
	if err != nil {
		return nil, errors.Wrapf(err, "could not format name %s", name)
	}
	session := m.session.Clone()
	defer session.Close()

	u := &User{}
	if err = session.DB(m.databaseName).C(m.collectionName).FindId(name).One(u); err != nil {
		if err == mgo.ErrNotFound {
			return nil, errors.Errorf("could not find %s in the database", name)
		}
		return nil, errors.Wrapf(err, "problem looking up %s in the database", name)
	}

	var data []byte
	switch key {
	case userCertKey:
		data = []byte(u.Cert)
	case userPrivateKeyKey:
		data = []byte(u.PrivateKey)
	case userCertReqKey:
		data = []byte(u.CertReq)
	case userCertRevocListKey:
		data = []byte(u.CertRevocList)
	}

	if len(data) == 0 {
		return nil, errors.New("no data available")
	}
	return data, nil
}

// Delete removes the data from a user specified by the tag.
func (m *mgoCertDepot) Delete(tag *depot.Tag) error {
	name, key, err := getNameAndKey(tag)
	if err != nil {
		return errors.Wrapf(err, "could not format name %s", name)
	}
	session := m.session.Clone()
	defer session.Close()

	update := bson.M{"$unset": bson.M{key: ""}}
	if err = m.session.DB(m.databaseName).C(m.collectionName).UpdateId(name, update); errNotNotFound(err) {
		return errors.Wrapf(err, "problem deleting %s.%s from the database", name, key)
	}

	return nil
}

func (m *mgoCertDepot) Save(name string, creds *Credentials) error { return depotSave(m, name, creds) }
func (m *mgoCertDepot) Find(name string) (*Credentials, error)     { return depotFind(m, name, m.opts) }
func (m *mgoCertDepot) Generate(name string) (*Credentials, error) {
	return depotGenerateDefault(m, name, m.opts)
}

func (m *mgoCertDepot) GenerateWithOptions(opts CertificateOptions) (*Credentials, error) {
	return depotGenerate(m, opts.CommonName, m.opts, opts)
}

func errNotNotFound(err error) bool {
	return err != nil && err != mgo.ErrNotFound
}
