package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/ftdc/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectJSONOptions(t *testing.T) {
	for _, test := range []struct {
		name  string
		valid bool
		opts  CollectJSONOptions
	}{
		{
			name:  "Nil",
			valid: false,
		},
		{
			name:  "FileWithIoReader",
			valid: false,
			opts: CollectJSONOptions{
				FileName:    "foo",
				InputSource: &bytes.Buffer{},
			},
		},
		{
			name:  "JustIoReader",
			valid: true,
			opts: CollectJSONOptions{
				InputSource: &bytes.Buffer{},
			},
		},
		{
			name:  "JustFile",
			valid: true,
			opts: CollectJSONOptions{
				FileName: "foo",
			},
		},
		{
			name:  "FileWithFollow",
			valid: true,
			opts: CollectJSONOptions{
				FileName: "foo",
				Follow:   true,
			},
		},
		{
			name:  "ReaderWithFollow",
			valid: false,
			opts: CollectJSONOptions{
				InputSource: &bytes.Buffer{},
				Follow:      true,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.valid {
				assert.NoError(t, test.opts.validate())
			} else {
				assert.Error(t, test.opts.validate())
			}
		})
	}
}

func makeJSONRandComplex(num int) ([][]byte, error) {
	out := [][]byte{}

	for i := 0; i < num; i++ {
		doc := testutil.RandComplexDocument(100, 2)
		data, err := json.Marshal(doc)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		out = append(out, data)
	}

	return out, nil
}

func writeStream(docs [][]byte, writer io.Writer) error {
	for _, doc := range docs {
		_, err := writer.Write(doc)
		if err != nil {
			return err
		}

		_, err = writer.Write([]byte("\n"))
		if err != nil {
			return err
		}
	}
	return nil
}

func TestCollectJSON(t *testing.T) {
	buildDir, err := filepath.Abs(filepath.Join("..", "build"))
	require.NoError(t, err)
	dir, err := ioutil.TempDir(buildDir, "ftdc-")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	defer func() {
		if err = os.RemoveAll(dir); err != nil {
			fmt.Println(err)
		}
	}()

	hundredDocs, err := makeJSONRandComplex(100)
	require.NoError(t, err)

	t.Run("SingleReaderIdealCase", func(t *testing.T) {
		buf := &bytes.Buffer{}
		err = writeStream(hundredDocs, buf)
		require.NoError(t, err)

		reader := bytes.NewReader(buf.Bytes())

		opts := CollectJSONOptions{
			OutputFilePrefix: filepath.Join(dir, fmt.Sprintf("json.%d.%s",
				os.Getpid(),
				time.Now().Format("2006-01-02.15-04-05"))),
			FlushInterval: 100 * time.Millisecond,
			SampleCount:   1000,
			InputSource:   reader,
		}

		err = CollectJSONStream(ctx, opts)
		assert.NoError(t, err)
	})
	t.Run("SingleReaderBotchedDocument", func(t *testing.T) {
		buf := &bytes.Buffer{}

		var docs [][]byte
		docs, err = makeJSONRandComplex(10)
		require.NoError(t, err)

		docs[2] = docs[len(docs)-1][1:] // break the last document

		err = writeStream(docs, buf)
		require.NoError(t, err)

		reader := bytes.NewReader(buf.Bytes())

		opts := CollectJSONOptions{
			OutputFilePrefix: filepath.Join(dir, fmt.Sprintf("json.%d.%s",
				os.Getpid(),
				time.Now().Format("2006-01-02.15-04-05"))),
			FlushInterval: 10 * time.Millisecond,
			InputSource:   reader,
			SampleCount:   100,
		}

		err = CollectJSONStream(ctx, opts)
		assert.Error(t, err)
	})
	t.Run("ReadFromFile", func(t *testing.T) {
		fn := filepath.Join(dir, "json-read-file-one")
		var f *os.File
		f, err = os.Create(fn)
		require.NoError(t, err)

		require.NoError(t, writeStream(hundredDocs, f))
		require.NoError(t, f.Close())

		opts := CollectJSONOptions{
			OutputFilePrefix: filepath.Join(dir, fmt.Sprintf("json.%d.%s",
				os.Getpid(),
				time.Now().Format("2006-01-02.15-04-05"))),
			FileName:    fn,
			SampleCount: 100,
		}

		err = CollectJSONStream(ctx, opts)
		assert.NoError(t, err)
	})
	t.Run("FollowFile", func(t *testing.T) {
		fn := filepath.Join(dir, "json-read-file-two")
		var f *os.File
		f, err = os.Create(fn)
		require.NoError(t, err)

		go func() {
			time.Sleep(10 * time.Millisecond)
			require.NoError(t, writeStream(hundredDocs, f))
			require.NoError(t, f.Close())
		}()

		ctx, cancel = context.WithTimeout(ctx, 250*time.Millisecond)
		defer cancel()
		opts := CollectJSONOptions{
			OutputFilePrefix: filepath.Join(dir, fmt.Sprintf("json.%d.%s",
				os.Getpid(),
				time.Now().Format("2006-01-02.15-04-05"))),
			SampleCount:   100,
			FlushInterval: 500 * time.Millisecond,
			FileName:      fn,
			Follow:        true,
		}

		err = CollectJSONStream(ctx, opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "operation aborted")
	})
	t.Run("RoundTrip", func(t *testing.T) {
		inputs := []map[string]interface{}{
			{
				"one":   int64(1),
				"other": int64(43),
			},
			{
				"one":   int64(33),
				"other": int64(41),
			},
			{
				"one":   int64(1),
				"other": int64(41),
			},
		}

		var (
			doc  []byte
			docs [][]byte
		)

		for _, in := range inputs {
			doc, err = json.Marshal(in)
			require.NoError(t, err)
			docs = append(docs, doc)
		}
		require.Len(t, docs, 3)

		buf := &bytes.Buffer{}

		require.NoError(t, writeStream(docs, buf))

		reader := bytes.NewReader(buf.Bytes())

		opts := CollectJSONOptions{
			OutputFilePrefix: filepath.Join(dir, "roundtrip"),
			FlushInterval:    time.Second,
			SampleCount:      50,
			InputSource:      reader,
		}
		ctx := context.Background()

		err = CollectJSONStream(ctx, opts)
		assert.NoError(t, err)
		_, err := os.Stat(filepath.Join(dir, "roundtrip.0"))
		require.False(t, os.IsNotExist(err))

		fn, err := os.Open(filepath.Join(dir, "roundtrip.0"))
		require.NoError(t, err)

		iter := ftdc.ReadMetrics(ctx, fn)
		idx := -1
		for iter.Next() {
			idx++

			s := iter.Document()
			assert.Equal(t, 2, s.Len())
			for k, v := range inputs[idx] {
				out := s.Lookup(k)
				assert.EqualValues(t, v, out.Interface())
			}
		}
		require.NoError(t, iter.Err())
		assert.Equal(t, 2, idx) // zero indexed
	})
}
