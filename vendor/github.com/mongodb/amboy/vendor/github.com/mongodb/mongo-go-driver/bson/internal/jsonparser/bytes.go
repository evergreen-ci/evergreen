// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package jsonparser

// About 3x faster then strconv.ParseInt because does not check for range error and support only base 10, which is enough for JSON
func parseInt(bytes []byte) (v int64, ok bool) {
	if len(bytes) == 0 {
		return 0, false
	}

	var neg bool = false
	if bytes[0] == '-' {
		neg = true
		bytes = bytes[1:]
	}

	for _, c := range bytes {
		if c >= '0' && c <= '9' {
			v = (10 * v) + int64(c-'0')
		} else {
			return 0, false
		}
	}

	if neg {
		return -v, true
	} else {
		return v, true
	}
}
