#!/usr/bin/env bash

# This script should be run from the root of the repository.
#
# Scans all files with a .go suffix and filters for files that are missing a
# Copyright notice at the top. Each detected file is then modified to include
# it.

tmpFile="./copyright_script.tmp"

for file in $(find . -name \*.go); do
	topLine=$(head -n 1 $file)
	if [[ $topLine != *"Copyright"* ]]; then
        echo -e "// Copyright 2025 Redpanda Data, Inc.\n" > $tmpFile
		cat $file >> $tmpFile
		cat $tmpFile > $file
	fi
done

rm -f $tmpFile
