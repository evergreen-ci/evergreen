package ftdc

import (
	"context"
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/ftdc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type metricHashFunc func(*birch.Document) (string, int)

func BenchmarkHashBSON(b *testing.B) {
	for _, impl := range []struct {
		Name     string
		HashFunc metricHashFunc
	}{
		{
			Name:     "FNVChecksum",
			HashFunc: metricKeyHash,
		},
	} {
		b.Run(impl.Name, func(b *testing.B) {
			for _, test := range []struct {
				Name string
				Doc  *birch.Document
			}{
				{
					Name: "FlatSmall",
					Doc:  testutil.RandFlatDocument(10),
				},
				{
					Name: "FlatLarge",
					Doc:  testutil.RandFlatDocument(100),
				},
				{
					Name: "ComplexSmall",
					Doc:  testutil.RandComplexDocument(10, 5),
				},
				{
					Name: "ComplexLarge",
					Doc:  testutil.RandComplexDocument(100, 5),
				},
				{
					Name: "MoreComplexSmall",
					Doc:  testutil.RandComplexDocument(10, 2),
				},
				{
					Name: "MoreComplexLarge",
					Doc:  testutil.RandComplexDocument(100, 2),
				},
				{
					Name: "EventMock",
					Doc:  testutil.CreateEventRecord(2, 2, 2, 2),
				},
			} {
				b.Run(test.Name, func(b *testing.B) {
					var (
						h   string
						num int
					)
					for n := 0; n < b.N; n++ {
						h, num = impl.HashFunc(test.Doc)
					}
					b.StopTimer()
					assert.NotZero(b, num)
					assert.NotZero(b, h)
				})
			}
		})
	}
}

func BenchmarkDocumentCreation(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, test := range []struct {
		Name      string
		Samples   int
		Length    int
		Reference *birch.Document
		Metrics   []Metric
	}{
		{
			Name:      "Flat",
			Samples:   1000,
			Length:    15,
			Reference: testutil.RandFlatDocument(15),
			Metrics:   produceMockMetrics(ctx, 1000, func() *birch.Document { return testutil.RandFlatDocument(15) }),
		},
		{
			Name:      "SmallFlat",
			Samples:   1000,
			Length:    5,
			Reference: testutil.RandFlatDocument(5),
			Metrics:   produceMockMetrics(ctx, 1000, func() *birch.Document { return testutil.RandFlatDocument(5) }),
		},
		{
			Name:      "LargeFlat",
			Samples:   1000,
			Length:    15,
			Reference: testutil.RandFlatDocument(15),
			Metrics:   produceMockMetrics(ctx, 1000, func() *birch.Document { return testutil.RandFlatDocument(100) }),
		},
		{
			Name:      "Complex",
			Samples:   1000,
			Length:    60,
			Reference: testutil.RandComplexDocument(20, 3),
			Metrics:   produceMockMetrics(ctx, 1000, func() *birch.Document { return testutil.RandComplexDocument(20, 3) }),
		},
		{
			Name:      "SmallComplex",
			Samples:   1000,
			Length:    10,
			Reference: testutil.RandComplexDocument(5, 1),
			Metrics:   produceMockMetrics(ctx, 1000, func() *birch.Document { return testutil.RandComplexDocument(5, 1) }),
		},
	} {
		var doc *birch.Document
		b.Run(test.Name, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				for i := 0; i < test.Samples; i++ {
					doc, _ = restoreDocument(test.Reference, i, test.Metrics, 0)
					require.NotNil(b, doc)
					require.Equal(b, test.Length, doc.Len())
				}
			}
		})
	}
}
