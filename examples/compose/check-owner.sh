#!/bin/sh
set -eu
project=$1
owner=$2
# Fail if Docker is unavailable. An empty owned project can safely be torn down.
containers=$(docker ps -aq --filter "label=com.docker.compose.project=$project")
for container in $containers; do
  actual=$(docker inspect --format '{{ index .Config.Labels "wtf.workspace" }}' "$container")
  [ "$actual" = "$owner" ] || exit 1
done
