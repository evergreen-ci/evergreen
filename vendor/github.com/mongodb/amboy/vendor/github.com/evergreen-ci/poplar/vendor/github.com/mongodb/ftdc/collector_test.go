package ftdc

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/ftdc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorInterface(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, collect := range createCollectors(ctx) {
		t.Run(collect.name, func(t *testing.T) {
			tests := createTests()

			for _, test := range tests {
				if testing.Short() {
					continue
				}

				if collect.uncompressed {
					t.Skip("not supported for uncompressed collectors")
				}

				t.Run(test.name, func(t *testing.T) {
					collector := collect.factory()

					assert.NoError(t, collector.SetMetadata(testutil.CreateEventRecord(42, int64(time.Minute), rand.Int63n(7), 4)))

					info := collector.Info()
					assert.Zero(t, info)

					for _, d := range test.docs {
						require.NoError(t, collector.Add(d))
					}
					info = collector.Info()

					if test.randStats {
						require.True(t, info.MetricsCount >= test.numStats,
							"%d >= %d", info.MetricsCount, test.numStats)
					} else {
						require.Equal(t, test.numStats, info.MetricsCount,
							"info=%+v, %v", info, test.docs)
					}

					out, err := collector.Resolve()
					if len(test.docs) > 0 {
						assert.NoError(t, err)
						assert.NotZero(t, out)
					} else {
						assert.Error(t, err)
						assert.Zero(t, out)
					}

					collector.Reset()
					info = collector.Info()
					assert.Zero(t, info)
				})
			}
			t.Run("ResolveWhenNil", func(t *testing.T) {
				collector := collect.factory()
				out, err := collector.Resolve()
				assert.Nil(t, out)
				assert.Error(t, err)
			})
			t.Run("RoundTrip", func(t *testing.T) {
				if collect.uncompressed {
					t.Skip("without compressing these tests don't make much sense")
				}
				for name, docs := range map[string][]*birch.Document{
					"Integers": []*birch.Document{
						testutil.RandFlatDocument(5),
						testutil.RandFlatDocument(5),
						testutil.RandFlatDocument(5),
						testutil.RandFlatDocument(5),
					},
					"DecendingHandIntegers": []*birch.Document{
						birch.NewDocument(birch.EC.Int64("one", 43), birch.EC.Int64("two", 5)),
						birch.NewDocument(birch.EC.Int64("one", 89), birch.EC.Int64("two", 4)),
						birch.NewDocument(birch.EC.Int64("one", 99), birch.EC.Int64("two", 3)),
						birch.NewDocument(birch.EC.Int64("one", 101), birch.EC.Int64("two", 2)),
					},
				} {
					t.Run(name, func(t *testing.T) {
						collector := collect.factory()
						count := 0
						for _, d := range docs {
							count++
							assert.NoError(t, collector.Add(d))
						}
						time.Sleep(time.Millisecond) // force context switch so that the buffered collector flushes
						info := collector.Info()
						require.Equal(t, info.SampleCount, count)

						out, err := collector.Resolve()
						require.NoError(t, err)
						buf := bytes.NewBuffer(out)

						iter := ReadStructuredMetrics(ctx, buf)
						idx := -1
						for iter.Next() {
							idx++
							t.Run(fmt.Sprintf("DocumentNumber_%d", idx), func(t *testing.T) {
								s := iter.Document()

								if !assert.Equal(t, fmt.Sprint(s), fmt.Sprint(docs[idx])) {
									fmt.Println("---", idx)
									fmt.Println("in: ", docs[idx])
									fmt.Println("out:", s)
								}
							})
						}
						assert.Equal(t, len(docs)-1, idx) // zero index
						require.NoError(t, iter.Err())

					})
				}
			})
		})
	}
}

func TestStreamingEncoding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, impl := range []struct {
		name    string
		factory func() (Collector, *bytes.Buffer)
	}{
		{
			name: "StreamingDynamic",
			factory: func() (Collector, *bytes.Buffer) {
				buf := &bytes.Buffer{}
				return NewStreamingDynamicCollector(100, buf), buf
			},
		},
		{
			name: "StreamingDynamicSmall",
			factory: func() (Collector, *bytes.Buffer) {
				buf := &bytes.Buffer{}
				return NewStreamingDynamicCollector(2, buf), buf
			},
		},
	} {
		t.Run(impl.name, func(t *testing.T) {
			for _, test := range createEncodingTests() {
				t.Run(test.name, func(t *testing.T) {
					t.Run("SingleValues", func(t *testing.T) {
						collector, buf := impl.factory()
						for _, val := range test.dataset {
							assert.NoError(t, collector.Add(birch.NewDocument(birch.EC.Int64("foo", val))))
						}
						require.NoError(t, FlushCollector(collector, buf))
						payload := buf.Bytes()

						iter := ReadMetrics(ctx, bytes.NewBuffer(payload))
						res := []int64{}
						idx := 0
						for iter.Next() {
							doc := iter.Document()
							require.NotNil(t, doc)
							val := doc.Lookup("foo").Int64()
							res = append(res, val)
							assert.Equal(t, val, test.dataset[idx])
							idx++
						}
						require.NoError(t, iter.Err())
						require.Equal(t, len(test.dataset), len(res))
						assert.Equal(t, test.dataset, res)
					})
					t.Run("MultipleValues", func(t *testing.T) {
						collector, buf := impl.factory()
						docs := []*birch.Document{}

						for _, val := range test.dataset {
							doc := birch.NewDocument(
								birch.EC.Int64("foo", val),
								birch.EC.Int64("dub", 2*val),
								birch.EC.Int64("dup", val),
								birch.EC.Int64("neg", -1*val),
								birch.EC.Int64("mag", 10*val),
							)
							docs = append(docs, doc)
							assert.NoError(t, collector.Add(doc))
						}

						require.NoError(t, FlushCollector(collector, buf))
						payload := buf.Bytes()

						iter := ReadMetrics(ctx, bytes.NewBuffer(payload))
						res := []int64{}
						for iter.Next() {
							doc := iter.Document()
							require.NotNil(t, doc)
							val := doc.Lookup("foo").Int64()
							res = append(res, val)
							idx := len(res) - 1

							assert.Equal(t, fmt.Sprint(doc), fmt.Sprint(docs[idx]))
						}

						require.NoError(t, iter.Err())
						require.Equal(t, len(test.dataset), len(res))
						assert.Equal(t, test.dataset, res)
					})

					t.Run("MultiValueKeyOrder", func(t *testing.T) {
						collector, buf := impl.factory()
						docs := []*birch.Document{}

						for idx, val := range test.dataset {
							var doc *birch.Document
							if len(test.dataset) >= 3 && (idx == 2 || idx == 3) {
								doc = birch.NewDocument(
									birch.EC.Int64("foo", val),
									birch.EC.Int64("mag", 10*val),
									birch.EC.Int64("neg", -1*val),
								)
							} else {
								doc = birch.NewDocument(
									birch.EC.Int64("foo", val),
									birch.EC.Int64("dub", 2*val),
									birch.EC.Int64("dup", val),
									birch.EC.Int64("neg", -1*val),
									birch.EC.Int64("mag", 10*val),
								)
							}

							docs = append(docs, doc)
							assert.NoError(t, collector.Add(doc))
						}
						require.NoError(t, FlushCollector(collector, buf))
						payload := buf.Bytes()

						iter := ReadMetrics(ctx, bytes.NewBuffer(payload))
						res := []int64{}
						for iter.Next() {
							doc := iter.Document()
							require.NotNil(t, doc)
							val := doc.Lookup("foo").Int64()
							res = append(res, val)
							idx := len(res) - 1

							assert.Equal(t, fmt.Sprint(doc), fmt.Sprint(docs[idx]))
						}

						require.NoError(t, iter.Err())
						require.Equal(t, len(test.dataset), len(res), "%v -> %v", test.dataset, res)
						assert.Equal(t, test.dataset, res)
					})
					t.Run("DifferentKeys", func(t *testing.T) {
						collector, buf := impl.factory()
						docs := []*birch.Document{}

						for idx, val := range test.dataset {
							var doc *birch.Document
							if len(test.dataset) >= 5 && (idx == 2 || idx == 3) {
								doc = birch.NewDocument(
									birch.EC.Int64("foo", val),
									birch.EC.Int64("dub", 2*val),
									birch.EC.Int64("dup", val),
									birch.EC.Int64("neg", -1*val),
									birch.EC.Int64("mag", 10*val),
								)
							} else {
								doc = birch.NewDocument(
									birch.EC.Int64("foo", val),
									birch.EC.Int64("mag", 10*val),
									birch.EC.Int64("neg", -1*val),
									birch.EC.Int64("dup", val),
									birch.EC.Int64("dub", 2*val),
								)
							}

							docs = append(docs, doc)
							assert.NoError(t, collector.Add(doc))
						}

						require.NoError(t, FlushCollector(collector, buf))
						payload := buf.Bytes()

						iter := ReadMetrics(ctx, bytes.NewBuffer(payload))
						res := []int64{}
						for iter.Next() {
							doc := iter.Document()
							require.NotNil(t, doc)
							val := doc.Lookup("foo").Int64()
							res = append(res, val)
							idx := len(res) - 1

							assert.Equal(t, fmt.Sprint(doc), fmt.Sprint(docs[idx]))
						}
						require.NoError(t, iter.Err())
						require.Equal(t, len(test.dataset), len(res), "%v -> %v", test.dataset, res)
						require.Equal(t, len(test.dataset), len(res))
					})
				})
			}
		})
	}
}

func TestFixedEncoding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, impl := range []struct {
		name    string
		factory func() Collector
	}{
		{
			name:    "Better",
			factory: func() Collector { return &betterCollector{maxDeltas: 20} },
		},
		{
			name:    "StableDynamic",
			factory: func() Collector { return NewDynamicCollector(100) },
		},
		{
			name:    "Streaming",
			factory: func() Collector { return newStreamingCollector(20, &bytes.Buffer{}) },
		},
	} {
		t.Run(impl.name, func(t *testing.T) {
			for _, test := range createEncodingTests() {
				t.Run(test.name, func(t *testing.T) {
					t.Run("SingleValues", func(t *testing.T) {
						collector := impl.factory()
						for _, val := range test.dataset {
							assert.NoError(t, collector.Add(birch.NewDocument(birch.EC.Int64("foo", val))))
						}

						payload, err := collector.Resolve()
						require.NoError(t, err)
						iter := ReadMetrics(ctx, bytes.NewBuffer(payload))
						res := []int64{}
						idx := 0
						for iter.Next() {
							doc := iter.Document()
							require.NotNil(t, doc)
							val := doc.Lookup("foo").Int64()
							res = append(res, val)
							assert.Equal(t, val, test.dataset[idx])
							idx++
						}
						require.NoError(t, iter.Err())
						require.Equal(t, len(test.dataset), len(res))
						assert.Equal(t, test.dataset, res)
					})
					t.Run("MultipleValues", func(t *testing.T) {
						collector := impl.factory()
						docs := []*birch.Document{}

						for _, val := range test.dataset {
							doc := birch.NewDocument(
								birch.EC.Int64("foo", val),
								birch.EC.Int64("dub", 2*val),
								birch.EC.Int64("dup", val),
								birch.EC.Int64("neg", -1*val),
								birch.EC.Int64("mag", 10*val),
							)
							docs = append(docs, doc)
							assert.NoError(t, collector.Add(doc))
						}

						payload, err := collector.Resolve()
						require.NoError(t, err)
						iter := ReadMetrics(ctx, bytes.NewBuffer(payload))
						res := []int64{}
						for iter.Next() {
							doc := iter.Document()
							require.NotNil(t, doc)
							val := doc.Lookup("foo").Int64()
							res = append(res, val)
							idx := len(res) - 1

							assert.Equal(t, fmt.Sprint(doc), fmt.Sprint(docs[idx]))
						}

						require.NoError(t, iter.Err())
						require.Equal(t, len(test.dataset), len(res))
						assert.Equal(t, test.dataset, res)
					})
				})
			}
			t.Run("SizeMismatch", func(t *testing.T) {
				collector := impl.factory()
				assert.NoError(t, collector.Add(birch.NewDocument(birch.EC.Int64("one", 43), birch.EC.Int64("two", 5))))
				assert.NoError(t, collector.Add(birch.NewDocument(birch.EC.Int64("one", 43), birch.EC.Int64("two", 5))))

				if strings.Contains(impl.name, "Dynamic") {
					assert.NoError(t, collector.Add(birch.NewDocument(birch.EC.Int64("one", 43))))
				} else {
					assert.Error(t, collector.Add(birch.NewDocument(birch.EC.Int64("one", 43))))
				}
			})
		})
	}
}

func TestCollectorSizeCap(t *testing.T) {
	for _, test := range []struct {
		name    string
		factory func() Collector
	}{
		{
			name:    "Better",
			factory: func() Collector { return &betterCollector{maxDeltas: 1} },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			collector := test.factory()
			assert.NoError(t, collector.Add(birch.NewDocument(birch.EC.Int64("one", 43), birch.EC.Int64("two", 5))))
			assert.NoError(t, collector.Add(birch.NewDocument(birch.EC.Int64("one", 43), birch.EC.Int64("two", 5))))
			assert.Error(t, collector.Add(birch.NewDocument(birch.EC.Int64("one", 43), birch.EC.Int64("two", 5))))
		})
	}
}

func TestWriter(t *testing.T) {
	t.Run("NilDocuments", func(t *testing.T) {
		collector := NewWriterCollector(2, &noopWriter{})
		_, err := collector.Write(nil)
		assert.Error(t, err)
		assert.NoError(t, collector.Close())
	})
	t.Run("RealDocument", func(t *testing.T) {
		collector := NewWriterCollector(2, &noopWriter{})
		doc, err := birch.NewDocument(birch.EC.Int64("one", 43), birch.EC.Int64("two", 5)).MarshalBSON()
		require.NoError(t, err)
		_, err = collector.Write(doc)
		assert.NoError(t, err)
		assert.NoError(t, collector.Close())
	})
	t.Run("CloseNoError", func(t *testing.T) {
		collector := NewWriterCollector(2, &noopWriter{})
		assert.NoError(t, collector.Close())
	})
	t.Run("CloseError", func(t *testing.T) {
		collector := NewWriterCollector(2, &errWriter{})
		doc, err := birch.NewDocument(birch.EC.Int64("one", 43), birch.EC.Int64("two", 5)).MarshalBSON()
		require.NoError(t, err)
		_, err = collector.Write(doc)
		require.NoError(t, err)
		assert.Error(t, collector.Close())
	})
}

func TestTimestampHandling(t *testing.T) {
	start := time.Now().Round(time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, test := range []struct {
		Name   string
		Values []time.Time
	}{
		{
			Name: "One",
			Values: []time.Time{
				time.Now().Round(time.Millisecond),
			},
		},
		{
			Name: "Same",
			Values: []time.Time{
				start, start, start,
			},
		},
		{
			Name: "SecondSteps",
			Values: []time.Time{
				start.Add(time.Second),
				start.Add(time.Second),
				start.Add(time.Second),
				start.Add(time.Second),
				start.Add(time.Second),
				start.Add(time.Second),
			},
		},
		{
			Name: "HundredMillis",
			Values: []time.Time{
				start.Add(100 * time.Millisecond),
				start.Add(200 * time.Millisecond),
				start.Add(300 * time.Millisecond),
				start.Add(400 * time.Millisecond),
				start.Add(500 * time.Millisecond),
				start.Add(600 * time.Millisecond),
			},
		},
		{
			Name: "TenMillis",
			Values: []time.Time{
				start.Add(10 * time.Millisecond),
				start.Add(20 * time.Millisecond),
				start.Add(30 * time.Millisecond),
				start.Add(40 * time.Millisecond),
				start.Add(50 * time.Millisecond),
				start.Add(60 * time.Millisecond),
			},
		},
	} {
		t.Run(test.Name, func(t *testing.T) {
			t.Run("TimeValue", func(t *testing.T) {
				collector := NewBaseCollector(100)
				for _, ts := range test.Values {
					require.NoError(t, collector.Add(birch.NewDocument(
						birch.EC.Time("ts", ts),
					)))
				}

				out, err := collector.Resolve()
				require.NoError(t, err)
				t.Run("Structured", func(t *testing.T) {
					iter := ReadStructuredMetrics(ctx, bytes.NewBuffer(out))
					idx := 0
					for iter.Next() {
						doc := iter.Document()

						val, ok := doc.Lookup("ts").TimeOK()
						if !assert.True(t, ok) {
							assert.Equal(t, test.Values[idx], val)
						}
						idx++
					}
					require.NoError(t, iter.Err())
				})
				t.Run("Flattened", func(t *testing.T) {
					iter := ReadMetrics(ctx, bytes.NewBuffer(out))
					idx := 0
					for iter.Next() {
						doc := iter.Document()
						val, ok := doc.Lookup("ts").TimeOK()
						if assert.True(t, ok) {
							assert.EqualValues(t, test.Values[idx], val)
						}
						idx++
					}
					require.NoError(t, iter.Err())
				})
				t.Run("Chunks", func(t *testing.T) {
					chunks := ReadChunks(ctx, bytes.NewBuffer(out))
					idx := 0
					for chunks.Next() {
						chunk := chunks.Chunk()
						assert.NotNil(t, chunk)
						assert.Equal(t, len(test.Values), chunk.nPoints)
						idx++
					}
					require.NoError(t, chunks.Err())
				})

			})
			t.Run("UnixSecond", func(t *testing.T) {
				collector := NewBaseCollector(100)
				for _, ts := range test.Values {
					require.NoError(t, collector.Add(birch.NewDocument(
						birch.EC.Int64("ts", ts.Unix()),
					)))
				}

				out, err := collector.Resolve()
				require.NoError(t, err)

				iter := ReadMetrics(ctx, bytes.NewBuffer(out))
				idx := 0
				for iter.Next() {
					doc := iter.Document()

					val, ok := doc.Lookup("ts").Int64OK()
					if assert.True(t, ok) {
						assert.Equal(t, test.Values[idx].Unix(), val)
					}
					idx++
				}
				require.NoError(t, iter.Err())
			})
			t.Run("UnixNano", func(t *testing.T) {
				collector := NewBaseCollector(100)
				for _, ts := range test.Values {
					require.NoError(t, collector.Add(birch.NewDocument(
						birch.EC.Int64("ts", ts.UnixNano()),
					)))
				}

				out, err := collector.Resolve()
				require.NoError(t, err)

				iter := ReadMetrics(ctx, bytes.NewBuffer(out))
				idx := 0
				for iter.Next() {
					doc := iter.Document()

					val, ok := doc.Lookup("ts").Int64OK()
					if assert.True(t, ok) {
						assert.Equal(t, test.Values[idx].UnixNano(), val)
					}

					idx++
				}
				require.NoError(t, iter.Err())
			})
		})
	}
}
