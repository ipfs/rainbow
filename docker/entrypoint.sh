#!/bin/sh

set -e
user=ipfs

if [ -n "$DOCKER_DEBUG" ]; then
   set -x
fi

if [ `id -u` -eq 0 ]; then
    echo "Changing user to $user"
    exec su-exec "$user" "$0" $@
fi

# Only ipfs user can get here
rainbow --version

exec rainbow $@
