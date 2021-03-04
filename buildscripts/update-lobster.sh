#!/bin/bash

# this script pulls the evergreen-ci/lobster repo and copies the files so that
# they can be served by evergreen
# it should only be run from the base evergreen directory
set -o errexit
set -o xtrace

SCRIPTS_DIR=buildscripts
LOBSTER_REPO=https://github.com/evergreen-ci/lobster.git
LOBSTER_DIR=.lobster-temp
LOBSTER_ASSETS_DIR=build
LOBSTER_STATIC_DIR=static
LOBSTER_HTML=index.html
EVERGREEN_STATIC_DIR=public/static/lobster
EVERGREEN_JS_DIR=js
EVERGREEN_CSS_DIR=css
EVERGREEN_TEMPLATES_DIR=service/templates

# clone lobster repo
pushd $SCRIPTS_DIR
rm -rf $LOBSTER_DIR
git clone $LOBSTER_REPO $LOBSTER_DIR

# build lobster
pushd $LOBSTER_DIR
npm install
npm run build 
popd
popd # cwd should be evergreen repo base now

LOBSTER_DIR_FROM_BASE="$SCRIPTS_DIR/$LOBSTER_DIR"

# replace existing js/css/html files in EVERGREEN with the updated ones
# delete files in evergreen repo
rm -rf $EVERGREEN_STATIC_DIR/$EVERGREEN_JS_DIR
rm -rf $EVERGREEN_STATIC_DIR/$EVERGREEN_CSS_DIR
# copy build files from lobster from lobster to evergreen
cp -R $LOBSTER_DIR_FROM_BASE/$LOBSTER_ASSETS_DIR/$LOBSTER_STATIC_DIR/ $EVERGREEN_STATIC_DIR/
cp $LOBSTER_DIR_FROM_BASE/$LOBSTER_ASSETS_DIR/$LOBSTER_HTML $EVERGREEN_TEMPLATES_DIR/

# surround the html with go template tags
pushd $EVERGREEN_TEMPLATES_DIR
echo pwd
echo -e "{{define \"base\"}}\n$(cat index.html)" > lobster.html
echo "{{end}}" >> lobster.html
popd
rm -rf $LOBSTER_DIR_FROM_BASE
echo "finished updating lobster"
