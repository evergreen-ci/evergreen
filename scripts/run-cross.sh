#!/bin/bash

set -o xtrace
set -o errexit

tar -zxvf dist-test.tar.gz

export EVGHOME=`pwd`/evergreen-test
export SETTINGS_OVERRIDE=`pwd`/gopath/src/github.com/evergreen-ci/evergreen/creds.yml
export EVERGREEN_ALL=true

$EVGHOME/bin/test.$(echo $1 | sed 's/-/./') --test.v --test.timeout=10m
