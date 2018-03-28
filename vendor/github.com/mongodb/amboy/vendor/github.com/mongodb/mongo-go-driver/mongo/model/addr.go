// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package model

import (
	"net"
	"strings"
)

const defaultPort = "27017"

// Addr is the address of a mongodb server.
type Addr string

// Network returns the network of the address.
func (a Addr) Network() string {
	if strings.HasSuffix(string(a), "sock") {
		return "unix"
	}
	return "tcp"
}

// String returns the canonical version of the address.
func (a Addr) String() string {
	// TODO: unicode case folding?
	s := strings.ToLower(string(a))
	if len(s) == 0 {
		return ""
	}
	if a.Network() != "unix" {
		_, _, err := net.SplitHostPort(s)
		if err != nil && strings.Contains(err.Error(), "missing port in address") {
			s += ":" + defaultPort
		}
	}

	return s
}

// Canonicalize creates a canonicalized address.
func (a Addr) Canonicalize() Addr {
	return Addr(a.String())
}
