package ftdc

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkIterator(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, test := range []struct {
		Name string
		Path string
	}{
		{
			Name: "PerfMockSmall",
			Path: "perf_metrics_small.ftdc",
		},
		{
			Name: "PerfMock",
			Path: "perf_metrics.ftdc",
		},
		{
			Name: "ServerStatus",
			Path: "metrics.ftdc",
		},
	} {
		b.Run(test.Name, func(b *testing.B) {
			file, err := os.Open(test.Path)
			require.NoError(b, err)
			defer func() {
				if err = file.Close(); err != nil {
					fmt.Println(err)
				}
			}()
			data, err := ioutil.ReadAll(file)
			require.NoError(b, err)
			b.ResetTimer()

			b.Run("Chunk", func(b *testing.B) {
				b.Run("Resolving", func(b *testing.B) {
					iter := ReadChunks(ctx, bytes.NewBuffer(data))
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						if !iter.Next() {
							break
						}
						require.NotNil(b, iter.Chunk())
					}
				})
				b.Run("Iterating", func(b *testing.B) {
					for n := 0; n < b.N; n++ {
						iter := ReadChunks(ctx, bytes.NewBuffer(data))
						for iter.Next() {
							require.NotNil(b, iter.Chunk())
						}
					}
				})
			})
			b.Run("Series", func(b *testing.B) {
				b.Run("Resolving", func(b *testing.B) {
					iter := ReadSeries(ctx, bytes.NewBuffer(data))
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						if !iter.Next() {
							break
						}
						require.NotNil(b, iter.Document())
					}
				})
				b.Run("Iterating", func(b *testing.B) {
					for n := 0; n < b.N; n++ {
						iter := ReadSeries(ctx, bytes.NewBuffer(data))
						for iter.Next() {
							require.NotNil(b, iter.Document())
						}
					}
				})
			})
			b.Run("Matrix", func(b *testing.B) {
				b.Run("Resolving", func(b *testing.B) {
					iter := ReadMatrix(ctx, bytes.NewBuffer(data))
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						if !iter.Next() {
							break
						}
						require.NotNil(b, iter.Document())
					}
				})
				b.Run("Iterating", func(b *testing.B) {
					for n := 0; n < b.N; n++ {
						iter := ReadMatrix(ctx, bytes.NewBuffer(data))
						for iter.Next() {
							require.NotNil(b, iter.Document())
						}
					}
				})
			})
			b.Run("Structured", func(b *testing.B) {
				b.Run("Resolving", func(b *testing.B) {
					iter := ReadStructuredMetrics(ctx, bytes.NewBuffer(data))
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						if !iter.Next() {
							break
						}
						require.NotNil(b, iter.Document())
					}
				})
				b.Run("Iterating", func(b *testing.B) {
					for n := 0; n < b.N; n++ {
						iter := ReadStructuredMetrics(ctx, bytes.NewBuffer(data))
						for iter.Next() {
							require.NotNil(b, iter.Document())
						}
					}
				})
			})
			b.Run("Flattened", func(b *testing.B) {
				b.Run("Resolving", func(b *testing.B) {
					iter := ReadStructuredMetrics(ctx, bytes.NewBuffer(data))
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						if !iter.Next() {
							break
						}
						require.NotNil(b, iter.Document())
					}
				})
				b.Run("Iterating", func(b *testing.B) {
					for n := 0; n < b.N; n++ {
						iter := ReadStructuredMetrics(ctx, bytes.NewBuffer(data))
						for iter.Next() {
							require.NotNil(b, iter.Document())
						}
					}
				})
			})
		})
	}
}
