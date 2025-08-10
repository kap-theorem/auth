#!/bin/bash
# Make wrapper for Git Bash
exec "$(dirname "$0")/make.sh" "$@"
