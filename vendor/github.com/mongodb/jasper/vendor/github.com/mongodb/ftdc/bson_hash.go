package ftdc

import (
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"hash"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/mongodb/ftdc/bsonx"
)

func metricKeyMD5(doc *bsonx.Document) (string, int) {
	checksum := md5.New()
	seen := metricKeyHashDocument(checksum, "", doc)
	return fmt.Sprintf("%x", checksum.Sum(nil)), seen
}

func metricKeySHA1(doc *bsonx.Document) (string, int) {
	checksum := sha1.New()
	seen := metricKeyHashDocument(checksum, "", doc)
	return fmt.Sprintf("%x", checksum.Sum(nil)), seen
}

func metricKeyHash(doc *bsonx.Document) (string, int) {
	checksum := fnv.New64()
	seen := metricKeyHashDocument(checksum, "", doc)
	return fmt.Sprintf("%x", checksum.Sum(nil)), seen
}

func metricKeyHashDocument(checksum hash.Hash, key string, doc *bsonx.Document) int {
	iter := doc.Iterator()
	seen := 0
	for iter.Next() {
		elem := iter.Element()
		seen += metricKeyHashValue(checksum, fmt.Sprintf("%s.%s", key, elem.Key()), elem.Value())
	}

	return seen
}

func metricKeyHashArray(checksum hash.Hash, key string, array *bsonx.Array) int {
	seen := 0
	iter := array.Iterator()
	idx := 0
	for iter.Next() {
		seen += metricKeyHashValue(checksum, fmt.Sprintf("%s.%d", key, idx), iter.Value())
		idx++
	}

	return seen
}

func metricKeyHashValue(checksum hash.Hash, key string, value *bsonx.Value) int {
	switch value.Type() {
	case bsonx.TypeArray:
		return metricKeyHashArray(checksum, key, value.MutableArray())
	case bsonx.TypeEmbeddedDocument:
		return metricKeyHashDocument(checksum, key, value.MutableDocument())
	case bsonx.TypeBoolean:
		checksum.Write([]byte(key))
		return 1
	case bsonx.TypeDouble:
		checksum.Write([]byte(key))
		return 1
	case bsonx.TypeInt32:
		checksum.Write([]byte(key))
		return 1
	case bsonx.TypeInt64:
		checksum.Write([]byte(key))
		return 1
	case bsonx.TypeDateTime:
		checksum.Write([]byte(key))
		return 1
	case bsonx.TypeTimestamp:
		checksum.Write([]byte(key))
		return 2
	default:
		return 0
	}
}

////////////////////////////////////////////////////////////////////////
//
// hashing functions for metrics-able documents

func metricsHash(doc *bsonx.Document) (string, int) {
	keys, num := isMetricsDocument("", doc)
	return strings.Join(keys, "\n"), num
}

func isMetricsDocument(key string, doc *bsonx.Document) ([]string, int) {
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

func isMetricsArray(key string, array *bsonx.Array) ([]string, int) {
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

func isMetricsValue(key string, val *bsonx.Value) ([]string, int) {
	switch val.Type() {
	case bsonx.TypeObjectID:
		return nil, 0
	case bsonx.TypeString:
		return nil, 0
	case bsonx.TypeDecimal128:
		return nil, 0
	case bsonx.TypeArray:
		return isMetricsArray(key, val.MutableArray())
	case bsonx.TypeEmbeddedDocument:
		return isMetricsDocument(key, val.MutableDocument())
	case bsonx.TypeBoolean:
		return []string{key}, 1
	case bsonx.TypeDouble:
		return []string{key}, 1
	case bsonx.TypeInt32:
		return []string{key}, 1
	case bsonx.TypeInt64:
		return []string{key}, 1
	case bsonx.TypeDateTime:
		return []string{key}, 1
	case bsonx.TypeTimestamp:
		return []string{key}, 2
	default:
		return nil, 0
	}
}
