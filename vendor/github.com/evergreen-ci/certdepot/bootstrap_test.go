package certdepot

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestBootstrapDepotConfigValidate(t *testing.T) {
	for _, test := range []struct {
		name string
		conf BootstrapDepotConfig
		fail bool
	}{
		{
			name: "ValidFileDepot",
			conf: BootstrapDepotConfig{
				FileDepot:   "depot",
				CAName:      "root",
				ServiceName: "localhost",
				CACert:      "ca cert",
				CAKey:       "ca key",
			},
		},
		{
			name: "ValidFileDepotWithNonNilMongoDepot",
			conf: BootstrapDepotConfig{
				FileDepot:   "depot",
				MongoDepot:  &MongoDBOptions{},
				CAName:      "root",
				ServiceName: "localhost",
				CACert:      "ca cert",
				CAKey:       "ca key",
			},
		},
		{
			name: "ValidMongoDepot",
			conf: BootstrapDepotConfig{
				MongoDepot: &MongoDBOptions{
					DatabaseName:   "one",
					CollectionName: "two",
				},
				CAName:      "root",
				ServiceName: "localhost",
				CACert:      "ca cert",
				CAKey:       "ca key",
			},
		},
		{
			name: "UnsetDepot",
			conf: BootstrapDepotConfig{
				CAName:      "root",
				ServiceName: "localhost",
				CACert:      "ca cert",
				CAKey:       "ca key",
			},
			fail: true,
		},
		{
			name: "UnsetDepotWithNonNilMongoDepot",
			conf: BootstrapDepotConfig{
				MongoDepot:  &MongoDBOptions{},
				CAName:      "root",
				ServiceName: "localhost",
				CACert:      "ca cert",
				CAKey:       "ca key",
			},
			fail: true,
		},
		{
			name: "MoreThanOneDepotSet",
			conf: BootstrapDepotConfig{
				FileDepot: "depot",
				MongoDepot: &MongoDBOptions{
					DatabaseName:   "one",
					CollectionName: "two",
				},
				CAName:      "root",
				ServiceName: "localhost",
				CACert:      "ca cert",
				CAKey:       "ca key",
			},
			fail: true,
		},
		{
			name: "NoCANameOrServiceName",
			conf: BootstrapDepotConfig{
				FileDepot: "depot",
				CACert:    "ca cert",
				CAKey:     "ca key",
			},
			fail: true,
		},
		{
			name: "NoCAName",
			conf: BootstrapDepotConfig{
				FileDepot:   "depot",
				ServiceName: "localhost",
				CACert:      "ca cert",
				CAKey:       "ca key",
			},
			fail: true,
		},
		{
			name: "NoServiceName",
			conf: BootstrapDepotConfig{
				FileDepot: "depot",
				CAName:    "root",
				CACert:    "ca cert",
				CAKey:     "ca key",
			},
			fail: true,
		},
		{
			name: "CACertSetCAKeyUnset",
			conf: BootstrapDepotConfig{
				FileDepot:   "depot",
				CAName:      "root",
				ServiceName: "localhost",
				CACert:      "ca cert",
			},
			fail: true,
		},
		{
			name: "CACertUnsetCAKeySet",
			conf: BootstrapDepotConfig{
				FileDepot:   "depot",
				CAName:      "root",
				ServiceName: "localhost",
				CAKey:       "ca key",
			},
			fail: true,
		},
		{
			name: "MismatchingCACommonName",
			conf: BootstrapDepotConfig{
				FileDepot:   "depot",
				CAName:      "root",
				ServiceName: "localhost",
				CAOpts:      &CertificateOptions{CommonName: "different"},
				ServiceOpts: &CertificateOptions{CommonName: "localhost"},
			},
			fail: true,
		},
		{
			name: "MismatchingServiceCommonName",
			conf: BootstrapDepotConfig{
				FileDepot:   "depot",
				CAName:      "root",
				ServiceName: "localhost",
				CAOpts:      &CertificateOptions{CommonName: "root"},
				ServiceOpts: &CertificateOptions{CommonName: "different"},
			},
			fail: true,
		},
		{
			name: "MismatchingServiceCA",
			conf: BootstrapDepotConfig{
				FileDepot:   "depot",
				CAName:      "root",
				ServiceName: "localhost",
				CAOpts:      &CertificateOptions{CommonName: "root"},
				ServiceOpts: &CertificateOptions{
					CommonName: "localhost",
					CA:         "different",
				},
			},
			fail: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.fail {
				assert.Error(t, test.conf.Validate())
			} else {
				assert.NoError(t, test.conf.Validate())
			}
		})
	}
}

func TestBootstrapDepot(t *testing.T) {
	depotName := "bootstrap_test"
	caName := "test_ca"
	serviceName := "test_service"
	databaseName := "certs"
	ctx := context.TODO()
	connctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	client, err := mongo.Connect(connctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err)
	tempDepot, err := NewFileDepot("temp_depot")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(depotName))
		assert.NoError(t, client.Database(databaseName).Drop(ctx))
		assert.NoError(t, os.RemoveAll("temp_depot"))
	}()

	opts := CertificateOptions{
		CommonName: caName,
		Expires:    time.Hour,
	}
	require.NoError(t, opts.Init(tempDepot))
	caCert, err := tempDepot.Get(CrtTag(caName))
	require.NoError(t, err)
	caKey, err := tempDepot.Get(PrivKeyTag(caName))
	require.NoError(t, err)

	for _, impl := range []struct {
		name          string
		setup         func(*BootstrapDepotConfig) Depot
		bootstrapFunc func(BootstrapDepotConfig) (Depot, error)
		tearDown      func()
	}{
		{
			name: "FileDepot",
			setup: func(conf *BootstrapDepotConfig) Depot {
				conf.FileDepot = depotName

				d, err := NewFileDepot(depotName)
				require.NoError(t, err)
				return d
			},
			bootstrapFunc: func(conf BootstrapDepotConfig) (Depot, error) {
				return BootstrapDepot(ctx, conf)
			},
			tearDown: func() {
				require.NoError(t, os.RemoveAll(depotName))
			},
		},
		{
			name: "MongoDepot",
			setup: func(conf *BootstrapDepotConfig) Depot {
				conf.MongoDepot = &MongoDBOptions{
					DatabaseName:   databaseName,
					CollectionName: depotName,
				}

				d, err := NewMongoDBCertDepot(ctx, conf.MongoDepot)
				require.NoError(t, err)
				return d
			},
			bootstrapFunc: func(conf BootstrapDepotConfig) (Depot, error) {
				return BootstrapDepot(ctx, conf)
			},
			tearDown: func() {
				require.NoError(t, client.Database(databaseName).Collection(depotName).Drop(ctx))
			},
		},
		{
			name: "MongoDepotExistingClient",
			setup: func(conf *BootstrapDepotConfig) Depot {
				conf.MongoDepot = &MongoDBOptions{
					DatabaseName:   databaseName,
					CollectionName: depotName,
				}

				d, err := NewMongoDBCertDepot(ctx, conf.MongoDepot)
				require.NoError(t, err)
				return d
			},
			bootstrapFunc: func(conf BootstrapDepotConfig) (Depot, error) {
				return BootstrapDepotWithMongoClient(ctx, client, conf)
			},
			tearDown: func() {
				require.NoError(t, client.Database(databaseName).Collection(depotName).Drop(ctx))
			},
		},
	} {
		t.Run(impl.name, func(t *testing.T) {
			for _, test := range []struct {
				name   string
				conf   BootstrapDepotConfig
				setup  func(Depot)
				test   func(Depot)
				hasErr bool
			}{
				{
					name: "ExistingCertsInDepot",
					conf: BootstrapDepotConfig{
						CAName:      caName,
						ServiceName: serviceName,
					},
					setup: func(d Depot) {
						assert.NoError(t, d.Put(CrtTag(caName), []byte("fake ca cert")))
						assert.NoError(t, d.Put(PrivKeyTag(caName), []byte("fake ca key")))
						assert.NoError(t, d.Put(CrtTag(serviceName), []byte("fake service cert")))
						assert.NoError(t, d.Put(PrivKeyTag(serviceName), []byte("fake service key")))
					},
					test: func(d Depot) {
						data, err := d.Get(CrtTag(caName))
						assert.NoError(t, err)
						assert.Equal(t, data, []byte("fake ca cert"))
						data, err = d.Get(PrivKeyTag(caName))
						assert.NoError(t, err)
						assert.Equal(t, data, []byte("fake ca key"))
						data, err = d.Get(CrtTag(serviceName))
						assert.NoError(t, err)
						assert.Equal(t, data, []byte("fake service cert"))
						data, err = d.Get(PrivKeyTag(serviceName))
						assert.NoError(t, err)
						assert.Equal(t, data, []byte("fake service key"))
					},
				},
				{
					name: "ExistingCAPassedIn",
					conf: BootstrapDepotConfig{
						CAName:      caName,
						ServiceName: serviceName,
						CACert:      string(caCert),
						CAKey:       string(caKey),
						ServiceOpts: &CertificateOptions{
							CommonName: serviceName,
							Host:       serviceName,
							CA:         caName,
							Expires:    time.Hour,
						},
					},
					test: func(d Depot) {
						data, err := d.Get(CrtTag(caName))
						assert.NoError(t, err)
						assert.Equal(t, data, caCert)
						data, err = d.Get(PrivKeyTag(caName))
						assert.NoError(t, err)
						assert.Equal(t, data, caKey)
					},
				},
				{
					name: "CertCreation",
					conf: BootstrapDepotConfig{
						CAName:      caName,
						ServiceName: serviceName,
						CAOpts: &CertificateOptions{
							CommonName: caName,
							Expires:    time.Hour,
						},
						ServiceOpts: &CertificateOptions{
							CommonName: serviceName,
							Host:       serviceName,
							CA:         caName,
							Expires:    time.Hour,
						},
					},
				},
				{
					name: "NilCAOpts",
					conf: BootstrapDepotConfig{
						CAName:      caName,
						ServiceName: serviceName,
						ServiceOpts: &CertificateOptions{
							CommonName: serviceName,
							Host:       serviceName,
							CA:         caName,
							Expires:    time.Hour,
						},
					},
					hasErr: true,
				},
				{
					name: "NilServiceOpts",
					conf: BootstrapDepotConfig{
						CAName:      caName,
						ServiceName: serviceName,
						CAOpts: &CertificateOptions{
							CommonName: caName,
							Expires:    time.Hour,
						},
					},
					hasErr: true,
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					implDepot := impl.setup(&test.conf)
					if test.setup != nil {
						test.setup(implDepot)
					}

					bd, err := impl.bootstrapFunc(test.conf)
					if test.hasErr {
						require.Error(t, err)
					} else {
						require.NoError(t, err)

						assert.True(t, bd.Check(CrtTag(caName)))
						assert.True(t, bd.Check(PrivKeyTag(caName)))
						assert.True(t, bd.Check(CrtTag(serviceName)))
						assert.True(t, bd.Check(PrivKeyTag(serviceName)))
					}

					if test.test != nil {
						test.test(bd)
					}
					impl.tearDown()
				})
			}
		})
	}
}
