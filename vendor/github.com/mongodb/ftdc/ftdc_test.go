package ftdc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/ftdc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Map converts the chunk to a map representation. Each key in the map
// is a "composite" key with a dot-separated fully qualified document
// path. The values in this map include all of the values collected
// for this chunk.
func (c *Chunk) renderMap() map[string]Metric {
	m := make(map[string]Metric)
	for _, metric := range c.Metrics {
		m[metric.Key()] = metric
	}
	return m
}

func printError(err error) {
	if err != nil {
		fmt.Println(err)
	}
}

type testMessage map[string]interface{}

func (m testMessage) String() string {
	by, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(by)
}

func TestReadPathIntegration(t *testing.T) {
	for _, test := range []struct {
		name            string
		path            string
		skipSlow        bool
		skipAll         bool
		expectedNum     int
		expectedChunks  int
		expectedMetrics int
		reportInterval  int
		docLen          int
	}{
		{
			name:            "PerfMock",
			path:            "perf_metrics.ftdc",
			docLen:          4,
			expectedNum:     10,
			expectedChunks:  10,
			expectedMetrics: 100000,
			reportInterval:  10000,
			skipSlow:        true,
		},
		{
			name:            "ServerStatus",
			path:            "metrics.ftdc",
			skipSlow:        true,
			docLen:          6,
			expectedNum:     1064,
			expectedChunks:  544,
			expectedMetrics: 300,
			reportInterval:  1000,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.skipAll && testing.Short() {
				t.Skip("skipping all read integration tests")
			}

			file, err := os.Open(test.path)
			require.NoError(t, err)
			defer func() { printError(file.Close()) }()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			data, err := ioutil.ReadAll(file)
			require.NoError(t, err)

			expectedSamples := test.expectedChunks * test.expectedMetrics
			t.Run("Chunks", func(t *testing.T) {
				startAt := time.Now()
				iter := ReadChunks(ctx, bytes.NewBuffer(data))
				counter := 0
				num := 0
				hasSeries := 0

				for iter.Next() {
					c := iter.Chunk()
					counter++
					if num == 0 {
						num = len(c.Metrics)
						require.Equal(t, test.expectedNum, num)
					}

					metric := c.Metrics[rand.Intn(num)]
					require.True(t, len(metric.Values) > 0)
					hasSeries++
					assert.Equal(t, metric.startingValue, metric.Values[0], "key=%s", metric.Key())
					assert.Len(t, metric.Values, test.expectedMetrics, "%d: %d", len(metric.Values), test.expectedMetrics)
					if counter == 10 {
						for _, v := range c.renderMap() {
							assert.Len(t, v.Values, test.expectedMetrics)
							assert.Equal(t, v.startingValue, v.Values[0], "key=%s", metric.Key())
						}

						numSamples := 0
						samples := c.Iterator(ctx)
						for samples.Next() {
							doc := samples.Document()

							numSamples++
							if assert.NotNil(t, doc) {
								assert.Equal(t, doc.Len(), test.expectedNum)
							}
						}
						assert.Equal(t, test.expectedMetrics, numSamples)

						data, err := c.export()
						require.NoError(t, err)

						assert.True(t, len(c.Metrics) >= data.Len())
						docIter := data.Iterator()
						elems := 0
						for docIter.Next() {
							array := docIter.Element().Value().MutableArray()
							require.Equal(t, test.expectedMetrics, array.Len())
							elems++
						}
						// this is inexact
						// because of timestamps...l
						assert.True(t, len(c.Metrics) >= elems)
						assert.Equal(t, elems, data.Len())
					}
				}

				assert.NoError(t, iter.Err())

				assert.Equal(t, test.expectedNum, num)
				assert.Equal(t, test.expectedChunks, counter)
				fmt.Println(testMessage{
					"parser":   "chunks",
					"series":   num,
					"iters":    counter,
					"dur_secs": time.Since(startAt).Seconds(),
				})
			})
			t.Run("MatrixSeries", func(t *testing.T) {
				startAt := time.Now()
				iter := ReadSeries(ctx, bytes.NewBuffer(data))
				counter := 0
				for iter.Next() {
					doc := iter.Document()
					require.NotNil(t, doc)
					require.True(t, doc.Len() > 0)
					counter++
				}
				assert.Equal(t, test.expectedChunks, counter)
				require.NoError(t, iter.Err())
				fmt.Println(testMessage{
					"parser":   "matrix_series",
					"iters":    counter,
					"dur_secs": time.Since(startAt).Seconds(),
				})
			})
			t.Run("Matrix", func(t *testing.T) {
				if test.skipSlow && testing.Short() {
					t.Skip("skipping slow read integration tests")
				}
				startAt := time.Now()
				iter := ReadMatrix(ctx, bytes.NewBuffer(data))
				counter := 0
				for iter.Next() {
					counter++

					if counter%10 != 0 {
						continue
					}

					doc := iter.Document()
					require.NotNil(t, doc)
					require.True(t, doc.Len() > 0)
				}
				require.NoError(t, iter.Err())
				assert.Equal(t, test.expectedChunks, counter)

				fmt.Println(testMessage{
					"parser":   "matrix",
					"iters":    counter,
					"dur_secs": time.Since(startAt).Seconds(),
				})
			})
			t.Run("Combined", func(t *testing.T) {
				if test.skipSlow && testing.Short() {
					t.Skip("skipping slow read integration tests")
				}

				t.Run("Structured", func(t *testing.T) {
					startAt := time.Now()
					iter := ReadStructuredMetrics(ctx, bytes.NewBuffer(data))
					counter := 0
					for iter.Next() {
						if counter >= expectedSamples/10 {
							break
						}

						counter++
						if counter%10 != 0 {
							continue
						}

						doc := iter.Document()
						require.NotNil(t, doc)
						if counter%test.reportInterval == 0 {
							fmt.Println(testMessage{
								"flavor":   "STRC",
								"seen":     counter,
								"elapsed":  time.Since(startAt).Seconds(),
								"metadata": iter.Metadata(),
							})
							startAt = time.Now()
						}

						require.Equal(t, test.docLen, doc.Len())
					}
					assert.NoError(t, iter.Err())
					assert.True(t, counter >= expectedSamples/10)

				})
				t.Run("Flattened", func(t *testing.T) {
					startAt := time.Now()
					iter := ReadMetrics(ctx, bytes.NewBuffer(data))
					counter := 0
					for iter.Next() {
						if counter >= expectedSamples/10 {
							break
						}
						counter++
						if counter%10 != 0 {
							continue
						}
						doc := iter.Document()
						require.NotNil(t, doc)
						if counter%test.reportInterval == 0 {
							fmt.Println(testMessage{
								"flavor":   "FLAT",
								"seen":     counter,
								"elapsed":  time.Since(startAt).Seconds(),
								"metadata": iter.Metadata(),
							})
							startAt = time.Now()
						}

						require.Equal(t, test.expectedNum, doc.Len())
					}
					assert.NoError(t, iter.Err())
					assert.True(t, counter >= expectedSamples/10)
				})
			})
		})
	}
}

func TestRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	collectors := createCollectors(ctx)
	for _, collect := range collectors {
		t.Run(collect.name, func(t *testing.T) {
			tests := createTests()
			for _, test := range tests {
				if test.numStats == 0 || (test.randStats && !strings.Contains(collect.name, "Dynamic")) {
					continue
				}
				if test.name != "Floats" {
					continue
				}
				t.Run(test.name, func(t *testing.T) {
					collector := collect.factory()
					assert.NoError(t, collector.SetMetadata(testutil.CreateEventRecord(42, int64(time.Minute), rand.Int63n(7), 4)))

					var docs []*birch.Document
					for _, d := range test.docs {
						assert.NoError(t, collector.Add(d))
						docs = append(docs, d)
					}

					data, err := collector.Resolve()
					require.NoError(t, err)
					iter := ReadStructuredMetrics(ctx, bytes.NewBuffer(data))

					docNum := 0
					for iter.Next() {
						require.True(t, docNum < len(docs))
						roundtripDoc := iter.Document()

						assert.Equal(t, fmt.Sprint(roundtripDoc), fmt.Sprint(docs[docNum]))
						docNum++
					}
				})
			}
		})
	}
}
