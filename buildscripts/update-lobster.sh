#!/bin/bash

# this script pulls the evergreen-ci/lobster repo and copies the files so that
# they can be served by evergreen
# it should only be run from the base evergreen directory
set -o errexit
set -o xtrace

LOBSTER_REPO=https://github.com/evergreen-ci/lobster.git
LOBSTER_TEMP_DIR="$EVGHOME/bin/.lobster-temp"
LOBSTER_TEMP_ASSETS_DIR="$LOBSTER_TEMP_DIR/build"
LOBSTER_STATIC_DIR="$LOBSTER_ASSETS_TEMP_DIR/static"
LOBSTER_HTML=index.html
EVG_LOB_MANIFEST_DIR="$EVGHOME/public/static/lobster"
EVG_LOB_STATIC_DIR="$EVG_LOB_MANIFEST_DIR/static"
EVERGREEN_TEMPLATES_DIR=service/templates

# clone lobster repo
git clone $LOBSTER_REPO $LOBSTER_TEMP_DIR

# build lobster
cd $LOBSTER_TEMP_DIR
npm install
# env variables used during lobster build step
export PUBLIC_URL=/static/lobster
export REACT_APP_LOGKEEPER_BASE=https://logkeeper.mongodb.org
export REACT_APP_EVERGREEN_BASE=https://evergreen.mongodb.com
#
npm run build 



# delete lobster static files in evergreen repo
rm -rf $EVG_LOB_STATIC_DIR
mkdir -p $EVG_LOB_STATIC_DIR

# copy files from lobster to evergreen
cp -R $LOBSTER_STATIC_DIR/ $EVG_LOB_STATIC_DIR/
cp "$LOBSTER_TEMP_ASSETS_DIR/$LOBSTER_HTML" "$EVERGREEN_TEMPLATES_DIR/temp.html"
cp "$LOBSTER_TEMP_ASSETS_DIR/manifest.json" $EVG_LOB_MANIFEST_DIR
# surround the html with go template tags
cd $EVERGREEN_TEMPLATES_DIR
echo -e "{{define \"base\"}}\n$(cat temp.html)" > lobster.html
echo "{{end}}" >> lobster.html
rm "$EVERGREEN_TEMPLATES_DIR/temp.html"
echo "finished updating lobster"
