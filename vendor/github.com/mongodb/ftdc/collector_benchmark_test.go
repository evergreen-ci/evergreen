package ftdc

import (
	"testing"
)

func BenchmarkCollectorInterface(b *testing.B) {
	collectors := createCollectors()
	for _, collect := range collectors {
		if collect.skipBench {
			continue
		}

		b.Run(collect.name, func(b *testing.B) {
			tests := createTests()
			for _, test := range tests {
				if test.skipBench {
					continue
				}

				b.Run(test.name, func(b *testing.B) {
					collector := collect.factory()
					b.Run("Add", func(b *testing.B) {
						for n := 0; n < b.N; n++ {
							collector.Add(test.docs[n%len(test.docs)])
						}
					})
					b.Run("Resolve", func(b *testing.B) {
						for n := 0; n < b.N; n++ {
							collector.Resolve()
						}
					})
				})
			}
		})
	}
}
