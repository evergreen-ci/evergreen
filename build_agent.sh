#!/bin/sh
set -o errexit

# make sure we're in the directory where the script lives
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR

# set up the $GOPATH appropriately
. ./set_gopath.sh
mkdir -p bin
export GOBIN=bin

GOXC="`pwd`/vendor/src/github.com/laher/goxc/goxc.go"
NONTASKS="go-vet,go-test,archive,rmbin"
DESTDIR="`pwd`/executables"
OSTARGETS="windows darwin linux solaris freebsd"
ARCHTARGETS="amd64 386"

# cd into the agent directory, and cross compile
cd agent/main
go run $GOXC -tasks-=$NONTASKS -d $DESTDIR -os="$OSTARGETS" -arch="$ARCHTARGETS"

# place the correct built agent githash into the appropriate file
git rev-parse HEAD > "$DESTDIR/version"
