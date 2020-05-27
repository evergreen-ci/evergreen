package ftdc

import (
	"fmt"
	"hash"
	"hash/fnv"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/birch/bsontype"
)

func metricKeyHash(doc *birch.Document) (string, int) {
	checksum := fnv.New64()
	seen := metricKeyHashDocument(checksum, "", doc)
	return fmt.Sprintf("%x", checksum.Sum(nil)), seen
}

func metricKeyHashDocument(checksum hash.Hash, key string, doc *birch.Document) int {
	iter := doc.Iterator()
	seen := 0
	for iter.Next() {
		elem := iter.Element()
		seen += metricKeyHashValue(checksum, fmt.Sprintf("%s.%s", key, elem.Key()), elem.Value())
	}

	return seen
}

func metricKeyHashArray(checksum hash.Hash, key string, array *birch.Array) int {
	seen := 0
	iter := array.Iterator()
	idx := 0
	for iter.Next() {
		seen += metricKeyHashValue(checksum, fmt.Sprintf("%s.%d", key, idx), iter.Value())
		idx++
	}

	return seen
}

func metricKeyHashValue(checksum hash.Hash, key string, value *birch.Value) int {
	switch value.Type() {
	case bsontype.Array:
		return metricKeyHashArray(checksum, key, value.MutableArray())
	case bsontype.EmbeddedDocument:
		return metricKeyHashDocument(checksum, key, value.MutableDocument())
	case bsontype.Boolean:
		_, _ = checksum.Write([]byte(key))
		return 1
	case bsontype.Double:
		_, _ = checksum.Write([]byte(key))
		return 1
	case bsontype.Int32:
		_, _ = checksum.Write([]byte(key))
		return 1
	case bsontype.Int64:
		_, _ = checksum.Write([]byte(key))
		return 1
	case bsontype.DateTime:
		_, _ = checksum.Write([]byte(key))
		return 1
	case bsontype.Timestamp:
		_, _ = checksum.Write([]byte(key))
		return 2
	default:
		return 0
	}
}
