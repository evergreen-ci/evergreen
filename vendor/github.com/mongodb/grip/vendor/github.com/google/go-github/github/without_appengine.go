// Copyright 2017 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !appengine
// +build go1.7

// This file provides glue for making github work without App Engine.

package github

import (
	"net/http"

	"golang.org/x/net/context"
)

func withContext(ctx context.Context, req *http.Request) (context.Context, *http.Request) {
	return ctx, req.WithContext(ctx)
}
