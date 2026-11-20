package testresult

import (
	"crypto/sha256"
	"encoding/binary"
	"slices"
)

const (
	// testNameFilterBitsPerTest and testNameFilterNumHashes give a false
	// positive rate of about 1% when the filter holds the number of tests it
	// was sized for.
	testNameFilterBitsPerTest = 10
	testNameFilterNumHashes   = 7
	// minTestNameFilterTests and maxTestNameFilterTests bound the number of
	// tests a filter is sized for, which keeps it between 80 bytes and 125KB.
	// A filter that holds more tests than it was sized for still works, but
	// with a higher false positive rate.
	minTestNameFilterTests = 64
	maxTestNameFilterTests = 100000
)

// TestNameFilter is a bloom filter of test names. It stores a set of test
// names in a small, fixed amount of space, at the cost of Contains sometimes
// reporting a test that was never added. It never misses a test that was
// added.
type TestNameFilter struct {
	Bits      []byte `bson:"bits"`
	NumHashes int    `bson:"num_hashes"`
}

// NewTestNameFilter returns an empty filter sized to hold the given number of
// tests.
func NewTestNameFilter(numTests int) *TestNameFilter {
	numTests = min(max(numTests, minTestNameFilterTests), maxTestNameFilterTests)
	return &TestNameFilter{
		Bits:      make([]byte, (numTests*testNameFilterBitsPerTest+7)/8),
		NumHashes: testNameFilterNumHashes,
	}
}

// Add adds the given test names to the filter.
func (f *TestNameFilter) Add(names ...string) {
	if len(f.Bits) == 0 {
		return
	}
	for _, name := range names {
		for _, bit := range f.bitIndices(name) {
			f.Bits[bit/8] |= 1 << (bit % 8)
		}
	}
}

// Contains returns whether the test name may have been added to the filter.
func (f *TestNameFilter) Contains(name string) bool {
	if len(f.Bits) == 0 || f.NumHashes <= 0 {
		return false
	}
	for _, bit := range f.bitIndices(name) {
		if f.Bits[bit/8]&(1<<(bit%8)) == 0 {
			return false
		}
	}
	return true
}

// Clone returns a copy of the filter.
func (f *TestNameFilter) Clone() *TestNameFilter {
	return &TestNameFilter{
		Bits:      slices.Clone(f.Bits),
		NumHashes: f.NumHashes,
	}
}

// bitIndices returns the bits that represent the test name, derived from two
// independent hashes of it (Kirsch-Mitzenmacher double hashing).
func (f *TestNameFilter) bitIndices(name string) []uint64 {
	sum := sha256.Sum256([]byte(name))
	h1 := binary.LittleEndian.Uint64(sum[0:8])
	h2 := binary.LittleEndian.Uint64(sum[8:16])
	numBits := uint64(len(f.Bits)) * 8
	indices := make([]uint64, f.NumHashes)
	for i := range indices {
		indices[i] = (h1 + uint64(i)*h2) % numBits
	}
	return indices
}
