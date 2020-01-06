#!/bin/bash

set -o errexit

echo "building aws creds file!"

if [ "Windows_NT" == "$OS" ]; then
  export AWS_DIR=$WORK_DIR/.aws
else
  export AWS_DIR=$HOME/.aws
fi
rm -rf $AWS_DIR
mkdir $AWS_DIR
cat <<EOF > $AWS_DIR/config
[default]
region = us-east-1
EOF
cat <<EOF > $AWS_DIR/credentials
[default]
aws_access_key_id = "$AWS_KEY"
aws_secret_access_key = "$AWS_SECRET"
EOF
