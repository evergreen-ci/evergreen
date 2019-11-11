package ftdc

import (
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"hash"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/birch/bsontype"
)

func metricKeyMD5(doc *birch.Document) (string, int) {
	checksum := md5.New()
	seen := metricKeyHashDocument(checksum, "", doc)
	return fmt.Sprintf("%x", checksum.Sum(nil)), seen
}

func metricKeySHA1(doc *birch.Document) (string, int) {
	checksum := sha1.New()
	seen := metricKeyHashDocument(checksum, "", doc)
	return fmt.Sprintf("%x", checksum.Sum(nil)), seen
}

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
		checksum.Write([]byte(key))
		return 1
	case bsontype.Double:
		checksum.Write([]byte(key))
		return 1
	case bsontype.Int32:
		checksum.Write([]byte(key))
		return 1
	case bsontype.Int64:
		checksum.Write([]byte(key))
		return 1
	case bsontype.DateTime:
		checksum.Write([]byte(key))
		return 1
	case bsontype.Timestamp:
		checksum.Write([]byte(key))
		return 2
	default:
		return 0
	}
}

////////////////////////////////////////////////////////////////////////
//
// hashing functions for metrics-able documents

func metricsHash(doc *birch.Document) (string, int) {
	keys, num := isMetricsDocument("", doc)
	return strings.Join(keys, "\n"), num
}

func isMetricsDocument(key string, doc *birch.Document) ([]string, int) {
	iter := doc.Iterator()
	keys := []string{}
	seen := 0
	for iter.Next() {
		elem := iter.Element()
		k, num := isMetricsValue(fmt.Sprintf("%s/%s", key, elem.Key()), elem.Value())
		if num > 0 {
			seen += num
			keys = append(keys, k...)
		}
	}

	return keys, seen
}

func isMetricsArray(key string, array *birch.Array) ([]string, int) {
	idx := 0
	numKeys := 0
	keys := []string{}
	iter := array.Iterator()
	for iter.Next() {
		ks, num := isMetricsValue(key+strconv.Itoa(idx), iter.Value())

		if num > 0 {
			numKeys += num
			keys = append(keys, ks...)
		}

		idx++
	}

	return keys, numKeys
}

func isMetricsValue(key string, val *birch.Value) ([]string, int) {
	switch val.Type() {
	case bsontype.ObjectID:
		return nil, 0
	case bsontype.String:
		return nil, 0
	case bsontype.Decimal128:
		return nil, 0
	case bsontype.Array:
		return isMetricsArray(key, val.MutableArray())
	case bsontype.EmbeddedDocument:
		return isMetricsDocument(key, val.MutableDocument())
	case bsontype.Boolean:
		return []string{key}, 1
	case bsontype.Double:
		return []string{key}, 1
	case bsontype.Int32:
		return []string{key}, 1
	case bsontype.Int64:
		return []string{key}, 1
	case bsontype.DateTime:
		return []string{key}, 1
	case bsontype.Timestamp:
		return []string{key}, 2
	default:
		return nil, 0
	}
}
