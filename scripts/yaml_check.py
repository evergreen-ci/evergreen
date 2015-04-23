#!/usr/bin/env python
import yaml
import sys
import fileinput

def main():
    for filename in sys.argv[1:]:
        print "validating: ", filename, "... ",
        yamlfile = open(filename, "r")
        yaml.load(file(filename, 'r'))
        print "OK."

if __name__ == "__main__":
    main()
