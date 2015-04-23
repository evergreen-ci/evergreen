#!/bin/bash

MCI_PKG='10gen.com/mci'

setgopath() {
    SOURCE_GOPATH=`pwd`.gopath
    VENDOR_GOPATH=`pwd`/vendor

    # set up the $GOPATH to use the vendored dependencies as
    # well as the source 
    rm -rf .gopath/
    mkdir -p .gopath/src/"$(dirname "${MCI_PKG}")"
    ln -sf `pwd` .gopath/src/$MCI_PKG
    export GOPATH=`pwd`/vendor:`pwd`/.gopath
}

setgopath
