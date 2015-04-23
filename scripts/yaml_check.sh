#!/bin/sh
find config_prod -name "*.yml" | xargs scripts/yaml_check.py
find config_dev -name "*.yml" | xargs scripts/yaml_check.py
find config_test -name "*.yml" | xargs scripts/yaml_check.py
