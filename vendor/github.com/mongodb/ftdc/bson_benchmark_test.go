package ftdc

import (
	"context"
	"testing"

	"github.com/evergreen-ci/birch"
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
			Name:     "Legacy",
			HashFunc: metricsHash,
		},
		{
			Name:     "FNVChecksum",
			HashFunc: metricKeyHash,
		},
		{
			Name:     "SHA1Checksum",
			HashFunc: metricKeySHA1,
		},
		{
			Name:     "MD5Checksum",
			HashFunc: metricKeyMD5,
		},
	} {
		b.Run(impl.Name, func(b *testing.B) {
			for _, test := range []struct {
				Name string
				Doc  *birch.Document
			}{
				{
					Name: "FlatSmall",
					Doc:  randFlatDocument(10),
				},
				{
					Name: "FlatLarge",
					Doc:  randFlatDocument(100),
				},
				{
					Name: "ComplexSmall",
					Doc:  randComplexDocument(10, 5),
				},
				{
					Name: "ComplexLarge",
					Doc:  randComplexDocument(100, 5),
				},
				{
					Name: "MoreComplexSmall",
					Doc:  randComplexDocument(10, 2),
				},
				{
					Name: "MoreComplexLarge",
					Doc:  randComplexDocument(100, 2),
				},
				{
					Name: "EventMock",
					Doc:  createEventRecord(2, 2, 2, 2),
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
			Reference: randFlatDocument(15),
			Metrics:   produceMockMetrics(ctx, 1000, func() *birch.Document { return randFlatDocument(15) }),
		},
		{
			Name:      "SmallFlat",
			Samples:   1000,
			Length:    5,
			Reference: randFlatDocument(5),
			Metrics:   produceMockMetrics(ctx, 1000, func() *birch.Document { return randFlatDocument(5) }),
		},
		{
			Name:      "LargeFlat",
			Samples:   1000,
			Length:    15,
			Reference: randFlatDocument(15),
			Metrics:   produceMockMetrics(ctx, 1000, func() *birch.Document { return randFlatDocument(100) }),
		},
		{
			Name:      "Complex",
			Samples:   1000,
			Length:    60,
			Reference: randComplexDocument(20, 3),
			Metrics:   produceMockMetrics(ctx, 1000, func() *birch.Document { return randComplexDocument(20, 3) }),
		},
		{
			Name:      "SmallComplex",
			Samples:   1000,
			Length:    10,
			Reference: randComplexDocument(5, 1),
			Metrics:   produceMockMetrics(ctx, 1000, func() *birch.Document { return randComplexDocument(5, 1) }),
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
