#!/bin/bash
exec "$(cd "$(dirname "$0")" && pwd)/dev" update "$@"
