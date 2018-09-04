package reporting

import (
	"testing"

	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	mgo "gopkg.in/mgo.v2"
)

func TestMongoDBConstructors(t *testing.T) {
	t.Run("NilSessionShouldError", func(t *testing.T) {
		db, err := MakeDBQueueState("foo", queue.DefaultMongoDBOptions(), nil)
		assert.Error(t, err)
		assert.Nil(t, db)
	})
	t.Run("UnpingableSessionError", func(t *testing.T) {
		opts := queue.DefaultMongoDBOptions()
		session, err := mgo.Dial(opts.URI)
		assert.NoError(t, err)
		session.Close()

		db, err := MakeDBQueueState("foo", opts, session)
		assert.Error(t, err)
		assert.Nil(t, db)
	})
	t.Run("BuildNewConnector", func(t *testing.T) {
		opts := queue.DefaultMongoDBOptions()
		session, err := mgo.Dial(opts.URI)
		assert.NoError(t, err)
		defer session.Close()

		db, err := MakeDBQueueState("foo", opts, session)
		assert.NoError(t, err)
		assert.NotNil(t, db)

		r, ok := db.(*dbQueueStat)
		assert.True(t, ok)

		assert.NotZero(t, r.collection)
	})
	t.Run("DialWithNewConstructor", func(t *testing.T) {
		r, err := NewDBQueueState("foo", queue.DefaultMongoDBOptions())
		assert.NoError(t, err)
		assert.NotNil(t, r)
		db := r.(*dbQueueStat)
		db.session.Close()
	})
	t.Run("DialWithBadURI", func(t *testing.T) {
		opts := queue.DefaultMongoDBOptions()
		opts.URI = "mongodb://lochost:26016"
		r, err := NewDBQueueState("foo", opts)
		assert.Error(t, err)
		assert.Nil(t, r)
	})
}
