#!/usr/bin/env bash

find . -type f -name '*.go' \
| xargs sed -i -e "s|// Code generated by gocc; DO NOT EDIT.|\
// Code generated by gocc; DO NOT EDIT.\n\
\n\
// This file is dual licensed under CC0 and The gonum license.\n\
//\n\
// Copyright ©2017 The gonum Authors. All rights reserved.\n\
// Use of this source code is governed by a BSD-style\n\
// license that can be found in the LICENSE file.\n\
//\n\
// Copyright ©2017 Robin Eklind.\n\
// This file is made available under a Creative Commons CC0 1.0\n\
// Universal Public Domain Dedication.\
|"

