# Gocloud 

[![Build Status](https://travis-ci.org/dynport/gocloud.png)](https://travis-ci.org/dynport/gocloud)

## Disclaimer

* __This is still a heavy work in progress. THINGS WILL CHANGE!__
* See the cli tool available in gocloud (e.g. gocloud/ec2.go) for examples.

## Goal

This should become a collection of golang libraries for all our beloved cloud service APIs (similar to fog).

I know that rackspace started gophercloud but somehow I could not find anything I could already use. So why not start my own?

## CLI tool

### Installation

    make


### Usage

Running `gocloud` without any arguments will give you  a list of currently supported actions

    make
    gocloud 

You can e.g. list your currently running ec2 instances with

    gocloud aws ec2 instances describe

You will get an error message if there are any environment variables not set. (all credentials are provided through ENV
variables).

## Contribute

All pull requests which make the api/code more consistent, dry, feature complete, etc. are always welcome.
