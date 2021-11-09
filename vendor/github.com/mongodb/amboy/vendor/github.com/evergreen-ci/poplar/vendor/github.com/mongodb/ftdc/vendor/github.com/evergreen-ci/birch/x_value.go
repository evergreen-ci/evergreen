package birch

import (
	"bytes"

	"github.com/evergreen-ci/birch/bsontype"
)

// EqualValue will return true if the two values are equal.
func checkEqualVal(t1, t2 bsontype.Type, v1, v2 []byte) bool {
	if t1 != t2 {
		return false
	}
	v1, _, ok := readValue(v1, t1)
	if !ok {
		return false
	}
	v2, _, ok = readValue(v2, t2)
	if !ok {
		return false
	}
	return bytes.Equal(v1, v2)
}

func readstring(src []byte) (string, []byte, bool) {
	l, rem, ok := readLength(src)
	if !ok {
		return "", src, false
	}
	if len(src[4:]) < int(l) {
		return "", src, false
	}

	return string(rem[:l-1]), rem[l:], true
}

func readLength(src []byte) (int32, []byte, bool) {
	if len(src) < 4 {
		return 0, src, false
	}

	return (int32(src[0]) | int32(src[1])<<8 | int32(src[2])<<16 | int32(src[3])<<24), src[4:], true
}

func appendi32(dst []byte, i32 int32) []byte {
	return append(dst, byte(i32), byte(i32>>8), byte(i32>>16), byte(i32>>24))
}

func appendstring(dst []byte, s string) []byte {
	l := int32(len(s) + 1)
	dst = appendi32(dst, l)
	dst = append(dst, s...)
	return append(dst, 0x00)
}

func readJavaScriptValue(src []byte) (js string, rem []byte, ok bool) { return readstring(src) }

// AppendCodeWithScope will append code and scope to dst and return the extended buffer.
func appendCodeWithScope(dst []byte, code string, scope []byte) []byte {
	length := int32(4 + 4 + len(code) + 1 + len(scope)) // length of cws, length of code, code, 0x00, scope
	dst = appendi32(dst, length)

	return append(appendstring(dst, code), scope...)
}
