#!/bin/bash

set -o xtrace
set -o errexit

rm -rf mongodb
mkdir -p mongodb
pushd mongodb
curl $1 -o mongodb.tgz

$2 mongodb.tgz
chmod +x ./mongodb-*/bin/*
mv ./mongodb-*/bin/* .
rm -rf db_files
rm -rf db_logs
mkdir -p db_files
mkdir -p db_logs
popd
