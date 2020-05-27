package poplar

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecorderType(t *testing.T) {
	for _, test := range []struct {
		Name    string
		Value   RecorderType
		Invalid bool
	}{
		{
			Name:  "Raw",
			Value: RecorderPerf,
		},
		{
			Name:  "Single",
			Value: RecorderPerfSingle,
		},
		{
			Name:  "GroupedShort",
			Value: RecorderPerf100ms,
		},
		{
			Name:  "GroupedLong",
			Value: RecorderPerf1s,
		},
		{
			Name:  "HistogramSingle",
			Value: RecorderHistogramSingle,
		},
		{
			Name:  "HistogramGroupedShort",
			Value: RecorderHistogram100ms,
		},
		{
			Name:  "HistogramGroupedLong",
			Value: RecorderHistogram1s,
		},
		{
			Name:    "Empty",
			Invalid: true,
		},
		{
			Name:    "Space",
			Value:   " ",
			Invalid: true,
		},
		{
			Name:    "Line",
			Value:   "\n",
			Invalid: true,
		},
		{
			Name:    "Close",
			Value:   "perf-invalid",
			Invalid: true,
		},
	} {
		t.Run(test.Name, func(t *testing.T) {
			if test.Invalid {
				require.Error(t, test.Value.Validate())
			} else {
				require.NoError(t, test.Value.Validate())
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "poplar-registry-")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(tmpdir))
	}()

	t.Run("GetNil", func(t *testing.T) {
		reg := NewRegistry()

		r, ok := reg.GetRecorder("foo")
		assert.False(t, ok)
		assert.Nil(t, r)

		c, ok := reg.GetCollector("foo")
		assert.False(t, ok)
		assert.Nil(t, c)
	})
	t.Run("CreateFailures", func(t *testing.T) {
		reg := NewRegistry()
		opts := CreateOptions{}

		r, err := reg.Create("foo", opts)
		assert.Error(t, err)
		assert.Nil(t, r)

		opts.Recorder = RecorderPerf
		r, err = reg.Create("foo", opts)
		assert.Error(t, err)
		assert.Nil(t, r)

		opts.Path = getPathOfFile()
		r, err = reg.Create("foo", opts)
		assert.Error(t, err)
		assert.Nil(t, r)
	})
	t.Run("Create", func(t *testing.T) {
		reg := NewRegistry()
		opts := CreateOptions{
			Recorder: RecorderPerf,
			Path:     filepath.Join(tmpdir, "bar"),
		}

		reg.cache["foo"] = &recorderInstance{}

		r, err := reg.Create("foo", opts)
		require.Error(t, err)
		assert.Nil(t, r)
		assert.Contains(t, err.Error(), "already exists")

		r, err = reg.Create("bar", opts)
		require.NoError(t, err)
		assert.NotNil(t, r)
		defer func() {
			impl, ok := reg.cache["bar"]
			if ok {
				assert.NoError(t, impl.file.Close())
			}
		}()

		_, err = reg.Create("bar", opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")

		rec, ok := reg.GetRecorder("bar")
		require.True(t, ok)
		require.NotNil(t, rec)
		assert.Equal(t, rec, r)

	})
	t.Run("Collector", func(t *testing.T) {
		reg := NewRegistry()
		opts := CreateOptions{
			Recorder: RecorderPerfSingle,
			Path:     filepath.Join(tmpdir, "baz"),
		}

		r, err := reg.Create("bar", opts)
		require.NoError(t, err)
		assert.NotNil(t, r)
		defer func() {
			impl, ok := reg.cache["bar"]
			if ok {
				assert.NoError(t, impl.file.Close())
			}
		}()

		coll, ok := reg.GetCollector("baz")
		require.False(t, ok)
		require.Nil(t, coll)

		coll, ok = reg.GetCollector("bar0")
		require.False(t, ok)
		require.Nil(t, coll)

		opts.Dynamic = true
		opts.Path = filepath.Join(tmpdir, "bz")

		r, err = reg.Create("foo", opts)
		require.NoError(t, err)
		assert.NotNil(t, r)
		defer func() {
			var impl *recorderInstance
			impl, ok = reg.cache["foo"]
			if ok {
				assert.NoError(t, impl.file.Close())
			}
		}()

		coll, ok = reg.GetCollector("foo")
		require.True(t, ok)
		assert.NotNil(t, coll)

		cus, ok := reg.GetCustomCollector("foo")
		require.False(t, ok)
		require.Nil(t, cus)

	})
	t.Run("CustomCollector", func(t *testing.T) {
		reg := NewRegistry()
		opts := CreateOptions{
			Recorder: CustomMetrics,
			Dynamic:  true,
			Path:     filepath.Join(tmpdir, "foobaz"),
		}
		rec, err := reg.Create("custom", opts)
		assert.NoError(t, err)
		assert.Nil(t, rec)
		defer func() {
			impl, ok := reg.cache["custom"]
			if ok {
				assert.NoError(t, impl.file.Close())
			}
		}()

		require.Error(t, ((*customEventTracker)(nil)).Add("foo", 1))

		coll, ok := reg.GetCustomCollector("not")
		require.Nil(t, coll)
		require.False(t, ok)

		coll, ok = reg.GetCustomCollector("custom")
		require.NotNil(t, coll)
		require.True(t, ok)
		require.NoError(t, coll.Add("foo", 1))
		require.Equal(t, 1, len(coll.Dump()))
		coll.Reset()
		require.Equal(t, 0, len(coll.Dump()))
	})
}
