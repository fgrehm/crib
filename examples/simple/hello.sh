#!/bin/sh
# This script is mounted into the container at the same path.
# After crib up, try: crib exec -- sh hello.sh
set -e

echo "Hello from inside the container!"
echo "CRIB_EXAMPLE=${CRIB_EXAMPLE}"
echo "Working directory: $(pwd)"
echo "Files in workspace:"
ls -la
