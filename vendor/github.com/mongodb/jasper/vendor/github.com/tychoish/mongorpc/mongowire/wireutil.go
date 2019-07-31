package mongowire

import "github.com/pkg/errors"

func readInt32(b []byte) int32 {
	return (int32(b[0])) |
		(int32(b[1]) << 8) |
		(int32(b[2]) << 16) |
		(int32(b[3]) << 24)
}

func readInt64(b []byte) int64 {
	return (int64(b[0])) |
		(int64(b[1]) << 8) |
		(int64(b[2]) << 16) |
		(int64(b[3]) << 24) |
		(int64(b[4]) << 32) |
		(int64(b[5]) << 40) |
		(int64(b[6]) << 48) |
		(int64(b[7]) << 56)
}

func writeInt32(i int32, buf []byte, loc int) {
	buf[loc] = byte(i)
	buf[loc+1] = byte(i >> 8)
	buf[loc+2] = byte(i >> 16)
	buf[loc+3] = byte(i >> 24)
}

func writeInt64(i int64, buf []byte, loc int) {
	buf[loc] = byte(i)
	buf[loc+1] = byte(i >> 8)
	buf[loc+2] = byte(i >> 16)
	buf[loc+3] = byte(i >> 24)
	buf[loc+4] = byte(i >> 32)
	buf[loc+5] = byte(i >> 40)
	buf[loc+6] = byte(i >> 48)
	buf[loc+7] = byte(i >> 56)
}

func readCString(b []byte) (string, error) {
	for i := 0; i < len(b); i++ {
		if b[i] == '\x00' {
			return string(b[0:i]), nil
		}
	}

	return "", errors.New("c string with no terminator")
}

func writeCString(s string, buf []byte, loc *int) {
	copy(buf[*loc:], s)
	*loc = *loc + len(s)
	buf[*loc] = 0
	*loc = *loc + 1
}
