package jsonx

// code in this file from: https://github.com/json-iterator/go/blob/master/stream_str.go

import "unicode/utf8"

var safeSet = [utf8.RuneSelf]bool{
	' ':      true,
	'!':      true,
	'"':      false,
	'#':      true,
	'$':      true,
	'%':      true,
	'&':      true,
	'\'':     true,
	'(':      true,
	')':      true,
	'*':      true,
	'+':      true,
	',':      true,
	'-':      true,
	'.':      true,
	'/':      true,
	'0':      true,
	'1':      true,
	'2':      true,
	'3':      true,
	'4':      true,
	'5':      true,
	'6':      true,
	'7':      true,
	'8':      true,
	'9':      true,
	':':      true,
	';':      true,
	'<':      true,
	'=':      true,
	'>':      true,
	'?':      true,
	'@':      true,
	'A':      true,
	'B':      true,
	'C':      true,
	'D':      true,
	'E':      true,
	'F':      true,
	'G':      true,
	'H':      true,
	'I':      true,
	'J':      true,
	'K':      true,
	'L':      true,
	'M':      true,
	'N':      true,
	'O':      true,
	'P':      true,
	'Q':      true,
	'R':      true,
	'S':      true,
	'T':      true,
	'U':      true,
	'V':      true,
	'W':      true,
	'X':      true,
	'Y':      true,
	'Z':      true,
	'[':      true,
	'\\':     false,
	']':      true,
	'^':      true,
	'_':      true,
	'`':      true,
	'a':      true,
	'b':      true,
	'c':      true,
	'd':      true,
	'e':      true,
	'f':      true,
	'g':      true,
	'h':      true,
	'i':      true,
	'j':      true,
	'k':      true,
	'l':      true,
	'm':      true,
	'n':      true,
	'o':      true,
	'p':      true,
	'q':      true,
	'r':      true,
	's':      true,
	't':      true,
	'u':      true,
	'v':      true,
	'w':      true,
	'x':      true,
	'y':      true,
	'z':      true,
	'{':      true,
	'|':      true,
	'}':      true,
	'~':      true,
	'\u007f': true,
}

const hex = "0123456789abcdef"

func writeJSONString(s []byte) []byte {
	out := make([]byte, 0, len(s))

	valLen := len(s)
	out = append(out, '"')

	// write string, the fast path, without utf8 and escape support
	i := 0
	for ; i < valLen; i++ {
		c := s[i]
		if c > 31 && c != '"' && c != '\\' {
			out = append(out, c)
		} else {
			break
		}
	}
	if i == valLen {
		return append(out, '"')
	}

	// Slow path...

	start := i
	// for the remaining parts, we process them char by char
	for i < valLen {
		if b := s[i]; b < utf8.RuneSelf {
			if safeSet[b] {
				i++
				continue
			}
			if start < i {
				out = append(out, []byte(s[start:i])...)
			}
			switch b {
			case '\\', '"':
				out = append(out, '\\', b)
			case '\n':
				out = append(out, '\\', 'n')
			case '\r':
				out = append(out, '\\', 'r')
			case '\t':
				out = append(out, '\\', 't')
			default:
				// This encodes bytes < 0x20 except for \t, \n and \r.
				// If escapeHTML is set, it also escapes <, >, and &
				// because they can lead to security holes when
				// user-controlled strings are rendered into JSON
				// and served to some browsers.
				out = append(out, []byte(`\u00`)...)
				out = append(out, hex[b>>4], hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		i++
		continue
	}
	if start < len(s) {
		out = append(out, []byte(s[start:])...)
	}
	out = append(out, []byte(s[start:])...)
	return append(out, '"')
}
