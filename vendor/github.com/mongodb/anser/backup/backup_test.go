package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type closableBuffer struct {
	bytes.Buffer
}

func (cb *closableBuffer) Close() error { return nil }

type fileCache map[string]*closableBuffer

func (mf fileCache) Target(ctx context.Context, name string) (io.WriteCloser, error) {
	if buf, ok := mf[name]; ok {
		return buf, nil
	}

	mf[name] = &closableBuffer{Buffer: bytes.Buffer{}}

	return mf[name], nil
}

func (mf fileCache) TargetErrors(context.Context, string) (io.WriteCloser, error) {
	return nil, errors.New("always")
}

func newDocument(doc *birch.Document, numKeys, otherNum int) *birch.Document {
	if doc == nil {
		doc = birch.DC.Make(numKeys * 3)
	}

	for i := 0; i < numKeys; i++ {
		doc.Append(birch.EC.Int64(fmt.Sprintln(numKeys, otherNum), rand.Int63n(int64(numKeys)*1)))
		doc.Append(birch.EC.Double(fmt.Sprintln("float", numKeys, otherNum), rand.Float64()))

		if otherNum%5 == 0 {
			ar := birch.NewArray()
			for ii := int64(0); i < otherNum; i++ {
				ar.Append(birch.VC.Int64(rand.Int63n(1 + ii*int64(numKeys))))
			}
			doc.Append(birch.EC.Array(fmt.Sprintln("first", numKeys, otherNum), ar))
		}

		if otherNum%3 == 0 {
			doc.Append(birch.EC.SubDocument(fmt.Sprintln("second", numKeys, otherNum), newDocument(nil, otherNum, otherNum)))
		}

		if otherNum%12 == 0 {
			doc.Append(birch.EC.SubDocument(fmt.Sprintln("third", numKeys, otherNum), newDocument(nil, otherNum, 2*otherNum)))
		}
	}

	return doc
}

func produceDocuments(doc *birch.Document, num int) []interface{} {
	out := make([]interface{}, num)

	if doc == nil {
		doc = birch.DC.New()
	}

	for idx := range out {
		out[idx] = newDocument(doc.Copy(), num, 10*num)
	}

	return out
}

func TestBackup(t *testing.T) {
	files := fileCache{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(time.Second))
	require.NoError(t, err)

	err = client.Connect(ctx)
	require.NoError(t, err)

	defer func() {
		client.Disconnect(ctx)
	}()

	t.Run("SimpleRoundTrip", func(t *testing.T) {
		defer func() { require.NoError(t, client.Database("foo").Collection("bar").Drop(ctx)) }()

		res, err := client.Database("foo").Collection("bar").InsertMany(ctx, produceDocuments(nil, 10))
		require.NoError(t, err)
		require.Len(t, res.InsertedIDs, 10)

		err = Collection(ctx, client, Options{
			NS:     model.Namespace{DB: "foo", Collection: "bar"},
			Target: files.Target,
		})
		require.NoError(t, err)
		require.Contains(t, files, "foo/bar.bson")
		require.Contains(t, files, "foo/bar.metadata.json")

		buf := files["foo/bar.bson"]
		count := 0
		for {
			doc, err := birch.DC.ReadFromErr(buf)
			if err == io.EOF {
				break
			}
			assert.NoError(t, err)
			if doc == nil {
				break
			}
			count++
		}
		assert.Equal(t, 10, count)
	})
	t.Run("Filter", func(t *testing.T) {
		defer func() { require.NoError(t, client.Database("foo").Collection("baz").Drop(ctx)) }()

		res, err := client.Database("foo").Collection("baz").InsertMany(ctx, produceDocuments(birch.DC.Elements(birch.EC.Int("a", 1)), 10))
		require.NoError(t, err)
		require.Len(t, res.InsertedIDs, 10)

		res, err = client.Database("foo").Collection("baz").InsertMany(ctx, produceDocuments(birch.DC.Elements(birch.EC.Int("b", 1)), 10))
		require.NoError(t, err)
		require.Len(t, res.InsertedIDs, 10)

		err = Collection(ctx, client, Options{
			NS:     model.Namespace{DB: "foo", Collection: "baz"},
			Target: files.Target,
			Query:  birch.DC.Elements(birch.EC.Int("a", 1)),
		})
		require.NoError(t, err)
		require.Contains(t, files, "foo/baz.bson")
		require.Contains(t, files, "foo/baz.metadata.json")

		count, err := client.Database("foo").Collection("baz").EstimatedDocumentCount(ctx)
		require.NoError(t, err)
		assert.EqualValues(t, count, 20)

		buf := files["foo/baz.bson"]
		count = 0
		for {
			doc, err := birch.DC.ReadFromErr(buf)
			if err == io.EOF {
				break
			}
			assert.NoError(t, err)
			if doc == nil {
				break
			}
			count++
		}
		assert.EqualValues(t, 10, count)
	})
	t.Run("Empty", func(t *testing.T) {
		defer func() { require.NoError(t, client.Database("foo").Collection("fuz").Drop(ctx)) }()

		err = Collection(ctx, client, Options{
			NS:     model.Namespace{DB: "foo", Collection: "fuz"},
			Target: files.Target,
		})
		require.NoError(t, err)
		require.Contains(t, files, "foo/fuz.bson")
		require.Contains(t, files, "foo/fuz.metadata.json")

		buf, ok := files["foo/fuz.bson"]
		require.True(t, ok)
		count := 0
		for {
			doc, err := birch.DC.ReadFromErr(buf)
			if err == io.EOF {
				break
			}
			assert.NoError(t, err)
			if doc == nil {
				break
			}
			count++
		}
		assert.EqualValues(t, 0, count)
	})
	t.Run("IndexesOnly", func(t *testing.T) {
		defer func() { require.NoError(t, client.Database("foo").Collection("bat").Drop(ctx)) }()

		res, err := client.Database("foo").Collection("bat").InsertMany(ctx, produceDocuments(nil, 10))
		require.NoError(t, err)
		require.Len(t, res.InsertedIDs, 10)

		err = Collection(ctx, client, Options{
			NS:          model.Namespace{DB: "foo", Collection: "bat"},
			Target:      files.Target,
			IndexesOnly: true,
		})
		require.NoError(t, err)
		require.NotContains(t, files, "foo/bat.bson")
		require.Contains(t, files, "foo/bat.metadata.json")

		doc := birch.DC.New()
		err = json.Unmarshal(files["foo/bat.metadata.json"].Buffer.Bytes(), doc)
		require.NoError(t, err)

		require.Equal(t, 0, doc.Lookup("options").MutableDocument().Len())
		require.Equal(t, 1, doc.Lookup("indexes").MutableArray().Len())
		require.Zero(t, doc.Lookup("uuid").StringValue())
	})
	t.Run("ProblmeGettingTarget", func(t *testing.T) {
		err = Collection(ctx, client, Options{
			NS:     model.Namespace{DB: "foo", Collection: "noop"},
			Target: files.TargetErrors,
		})
		require.Error(t, err)
		require.NotContains(t, files, "foo/noop.bson")
		require.NotContains(t, files, "foo/noop.metadata.json")
	})
	t.Run("QueryOptions", func(t *testing.T) {
		opts := &Options{
			Sort:  birch.DC.Elements(birch.EC.Int("a", 1)),
			Limit: 200,
		}
		assert.Nil(t, opts.Query)
		qopts := opts.getQueryOpts()
		assert.NotNil(t, opts.Query)
		assert.Equal(t, struct{}{}, opts.Query)
		assert.EqualValues(t, 200, *qopts.Limit)
		assert.EqualValues(t, opts.Sort, qopts.Sort)

	})

}
