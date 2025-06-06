#!/usr/bin/env bash

find $(dirname $0) -maxdepth 1 -mindepth 1 -type d -name "v1alpha2" -exec {}/e2e-tests.sh "$@" \;
