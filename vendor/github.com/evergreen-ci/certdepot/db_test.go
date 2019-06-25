package certdepot

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestDB(t *testing.T) {
	const (
		databaseName   = "certDepot"
		collectionName = "certs"
		dbTimeout      = 5 * time.Second
	)
	for name, testCase := range map[string]func(ctx context.Context, t *testing.T, md *mongoDepot, client *mongo.Client, coll *mongo.Collection){
		"PutTTL": func(ctx context.Context, t *testing.T, md *mongoDepot, client *mongo.Client, coll *mongo.Collection) {
			for subTestName, subTestCase := range map[string]func(ctx context.Context, t *testing.T){
				"SetsValueOnExistingDocument": func(ctx context.Context, t *testing.T) {
					name := "user"
					user := &User{
						ID:            name,
						Cert:          "cert",
						PrivateKey:    "key",
						CertReq:       "certReq",
						CertRevocList: "certRevocList",
					}
					_, err := coll.InsertOne(ctx, user)
					require.NoError(t, err)
					ttl := time.Now()
					require.NoError(t, md.PutTTL(name, ttl))

					user.TTL = ttl
					dbUser := &User{}
					require.NoError(t, coll.FindOne(ctx, bson.M{userIDKey: name}).Decode(dbUser))
					assert.Equal(t, user.ID, dbUser.ID)
				},
				"DoesNotInsert": func(ctx context.Context, t *testing.T) {
					name := "user"
					ttl := time.Now()
					require.Error(t, md.PutTTL(name, ttl))
					dbUser := &User{}
					assert.Equal(t, mongo.ErrNoDocuments, coll.FindOne(ctx, bson.M{userIDKey: name}).Decode(dbUser))
				},
			} {
				t.Run(subTestName, func(t *testing.T) {
					require.NoError(t, coll.Drop(ctx))
					defer func() {
						require.NoError(t, coll.Drop(ctx))
					}()
					tctx, cancel := context.WithTimeout(ctx, dbTimeout)
					defer cancel()
					subTestCase(tctx, t)
				})
			}
		},
		"FindExpiresBefore": func(ctx context.Context, t *testing.T, md *mongoDepot, client *mongo.Client, coll *mongo.Collection) {
			for subTestName, subTestCase := range map[string]func(ctx context.Context, t *testing.T){
				"MatchesExpired": func(ctx context.Context, t *testing.T) {
					name1 := "user1"
					name2 := "user2"
					ttl := time.Now()
					expiration := ttl.Add(time.Hour)
					userBeforeExpiration := &User{
						ID:  name1,
						TTL: ttl,
					}
					userAfterExpiration := &User{
						ID:  name2,
						TTL: expiration.Add(time.Hour),
					}

					_, err := coll.InsertOne(ctx, userBeforeExpiration)
					require.NoError(t, err)
					_, err = coll.InsertOne(ctx, userAfterExpiration)
					require.NoError(t, err)
					dbUsers, err := md.FindExpiresBefore(expiration)
					require.NoError(t, err)
					require.Len(t, dbUsers, 1)
					assert.Equal(t, userBeforeExpiration.ID, dbUsers[0].ID)
				},
				"IgnoresDocumentsWithoutTTL": func(ctx context.Context, t *testing.T) {
					name := "user"
					user := &User{
						ID:            name,
						Cert:          "cert",
						PrivateKey:    "key",
						CertReq:       "certReq",
						CertRevocList: "certRevocList",
					}
					_, err := coll.InsertOne(ctx, user)
					require.NoError(t, err)
					dbUsers, err := md.FindExpiresBefore(time.Now())
					require.NoError(t, err)
					assert.Empty(t, dbUsers)
				},
			} {
				t.Run(subTestName, func(t *testing.T) {
					require.NoError(t, coll.Drop(ctx))
					defer func() {
						require.NoError(t, coll.Drop(ctx))
					}()
					tctx, cancel := context.WithTimeout(ctx, dbTimeout)
					defer cancel()
					subTestCase(tctx, t)
				})
			}
		},
		"DeleteExpiresBefore": func(ctx context.Context, t *testing.T, md *mongoDepot, client *mongo.Client, coll *mongo.Collection) {
			for subTestName, subTestCase := range map[string]func(ctx context.Context, t *testing.T){
				"MatchesExpired": func(ctx context.Context, t *testing.T) {
					name1 := "user1"
					name2 := "user2"
					ttl := time.Now()
					expiration := ttl.Add(time.Hour)
					userBeforeExpiration := &User{
						ID:  name1,
						TTL: ttl,
					}
					userAfterExpiration := &User{
						ID:  name2,
						TTL: expiration.Add(time.Hour),
					}

					_, err := coll.InsertOne(ctx, userBeforeExpiration)
					require.NoError(t, err)
					_, err = coll.InsertOne(ctx, userAfterExpiration)
					require.NoError(t, err)
					require.NoError(t, md.DeleteExpiresBefore(expiration))
					dbUsers := []User{}
					res, err := coll.Find(ctx, bson.M{})
					require.NoError(t, err)
					require.NoError(t, res.All(ctx, &dbUsers))
					require.Len(t, dbUsers, 1)
					assert.Equal(t, userAfterExpiration.ID, dbUsers[0].ID)
				},
				"IgnoresDocumentsWithoutTTL": func(ctx context.Context, t *testing.T) {
					name := "user"
					user := &User{
						ID:            name,
						Cert:          "cert",
						PrivateKey:    "key",
						CertReq:       "certReq",
						CertRevocList: "certRevocList",
					}
					_, err := coll.InsertOne(ctx, user)
					require.NoError(t, err)
					require.NoError(t, md.DeleteExpiresBefore(time.Now()))
					count, err := coll.CountDocuments(ctx, bson.M{})
					require.NoError(t, err)
					assert.EqualValues(t, 1, count)
				},
			} {
				t.Run(subTestName, func(t *testing.T) {
					require.NoError(t, coll.Drop(ctx))
					defer func() {
						require.NoError(t, coll.Drop(ctx))
					}()
					tctx, cancel := context.WithTimeout(ctx, dbTimeout)
					defer cancel()
					subTestCase(tctx, t)
				})
			}
		},
	} {

		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			uri := "mongodb://localhost:27017"
			client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
			require.NoError(t, err)

			opts := &MongoDBOptions{
				MongoDBURI:     uri,
				DatabaseName:   databaseName,
				CollectionName: collectionName,
			}
			d, err := NewMongoDBCertDepotWithClient(ctx, client, opts)
			require.NoError(t, err)
			md, ok := d.(*mongoDepot)
			require.True(t, ok)

			testCase(ctx, t, md, client, client.Database(databaseName).Collection(collectionName))
		})
	}
}
