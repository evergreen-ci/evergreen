#!/bin/bash -x

go get github.com/dynport/gocloud/aws/pricing/aws-dump-instance-types

aws-dump-instance-types > instance_types.json

base_url=http://aws-assets-pricing-prod.s3.amazonaws.com/pricing/ec2

for name in linux-ri-heavy linux-ri-medium linux-ri-light linux-od; do
  echo "fetching $name"
  curl -s $base_url/$name.js | sed 's/^callback.*//' | sed 's/^)//' > ${name}.json
done
