// Copyright ©2015 The gonum Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//+build !appengine

package mat

import "unsafe"

// offset returns the number of float64 values b[0] is after a[0].
func offset(a, b []float64) int {
	if &a[0] == &b[0] {
		return 0
	}
	// This expression must be atomic with respect to GC moves.
	// At this stage this is true, because the GC does not
	// move. See https://golang.org/issue/12445.
	return int(uintptr(unsafe.Pointer(&b[0]))-uintptr(unsafe.Pointer(&a[0]))) / int(unsafe.Sizeof(float64(0)))
}
