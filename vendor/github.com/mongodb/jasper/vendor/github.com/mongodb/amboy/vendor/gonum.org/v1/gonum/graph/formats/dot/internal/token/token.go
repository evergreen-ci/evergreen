// Code generated by gocc; DO NOT EDIT.

// This file is dual licensed under CC0 and The gonum license.
//
// Copyright ©2017 The gonum Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Copyright ©2017 Robin Eklind.
// This file is made available under a Creative Commons CC0 1.0
// Universal Public Domain Dedication.

package token

import (
	"fmt"
)

type Token struct {
	Type
	Lit []byte
	Pos
}

type Type int

const (
	INVALID Type = iota
	EOF
)

type Pos struct {
	Offset int
	Line   int
	Column int
}

func (p Pos) String() string {
	return fmt.Sprintf("Pos(offset=%d, line=%d, column=%d)", p.Offset, p.Line, p.Column)
}

type TokenMap struct {
	typeMap []string
	idMap   map[string]Type
}

func (m TokenMap) Id(tok Type) string {
	if int(tok) < len(m.typeMap) {
		return m.typeMap[tok]
	}
	return "unknown"
}

func (m TokenMap) Type(tok string) Type {
	if typ, exist := m.idMap[tok]; exist {
		return typ
	}
	return INVALID
}

func (m TokenMap) TokenString(tok *Token) string {
	//TODO: refactor to print pos & token string properly
	return fmt.Sprintf("%s(%d,%s)", m.Id(tok.Type), tok.Type, tok.Lit)
}

func (m TokenMap) StringType(typ Type) string {
	return fmt.Sprintf("%s(%d)", m.Id(typ), typ)
}

var TokMap = TokenMap{
	typeMap: []string{
		"INVALID",
		"$",
		"{",
		"}",
		"empty",
		"strict",
		"graphx",
		"digraph",
		";",
		"--",
		"->",
		"node",
		"edge",
		"[",
		"]",
		",",
		"=",
		"subgraph",
		":",
		"id",
	},

	idMap: map[string]Type{
		"INVALID":  0,
		"$":        1,
		"{":        2,
		"}":        3,
		"empty":    4,
		"strict":   5,
		"graphx":   6,
		"digraph":  7,
		";":        8,
		"--":       9,
		"->":       10,
		"node":     11,
		"edge":     12,
		"[":        13,
		"]":        14,
		",":        15,
		"=":        16,
		"subgraph": 17,
		":":        18,
		"id":       19,
	},
}
