package birch

import (
	"bytes"

	"github.com/evergreen-ci/birch/bsontype"
)

func readValue(src []byte, t bsontype.Type) ([]byte, []byte, bool) {
	var length int32
	ok := true
	switch t {
	case bsontype.Array, bsontype.EmbeddedDocument, bsontype.CodeWithScope:
		length, _, ok = readLength(src)
	case bsontype.Binary:
		length, _, ok = readLength(src)
		length += 4 + 1 // binary length + subtype byte
	case bsontype.Boolean:
		length = 1
	case bsontype.DBPointer:
		length, _, ok = readLength(src)
		length += 4 + 12 // string length + ObjectID length
	case bsontype.DateTime, bsontype.Double, bsontype.Int64, bsontype.Timestamp:
		length = 8
	case bsontype.Decimal128:
		length = 16
	case bsontype.Int32:
		length = 4
	case bsontype.JavaScript, bsontype.String, bsontype.Symbol:
		length, _, ok = readLength(src)
		length += 4
	case bsontype.MaxKey, bsontype.MinKey, bsontype.Null, bsontype.Undefined:
		length = 0
	case bsontype.ObjectID:
		length = 12
	case bsontype.Regex:
		regex := bytes.IndexByte(src, 0x00)
		if regex < 0 {
			ok = false
			break
		}
		pattern := bytes.IndexByte(src[regex+1:], 0x00)
		if pattern < 0 {
			ok = false
			break
		}
		length = int32(int64(regex) + 1 + int64(pattern) + 1)
	default:
		ok = false
	}

	if !ok || int(length) > len(src) {
		return nil, src, false
	}

	return src[:length], src[length:], true
}
