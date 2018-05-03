#!/bin/bash

set -o xtrace
set -o errexit

rm -rf mongodb
mkdir mongodb
cd mongodb
curl $1 -o mongodb.tgz

$2 mongodb.tgz
chmod +x ./mongodb-*/bin/*
mv ./mongodb-*/bin/* .
rm -rf db_files
rm -rf db_logs
mkdir db_files
mkdir db_logs
