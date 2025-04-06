#!/bin/bash

TODAY=$(date +%Y%m%d)
EXISTING_TAGS=$(git tag -l "${TODAY}V*")
if [[ -z "$EXISTING_TAGS" ]]; then
    NEXT_VERSION="${TODAY}V001"
else
    LAST_TAG=$(echo "$EXISTING_TAGS" | sort | tail -n 1)
    LAST_INCREMENT=$(echo "$LAST_TAG" | sed -E "s/${TODAY}V([0-9]{3})/\1/")
    NEXT_INCREMENT=$(printf "%03d" $((10#$LAST_INCREMENT + 1)))
    NEXT_VERSION="${TODAY}V${NEXT_INCREMENT}"
fi

# Set the new version
git tag "$NEXT_VERSION"

VERSION=$(git describe --tags --abbrev=0)
COMMIT=$(git rev-parse --short HEAD)
DATE=$(date +%Y-%m-%dT%H:%M:%S%z)
if [[ -z "$VERSION" ]]; then
    echo "Error: Unable to retrieve version from git tags."
    exit 1
fi

if [[ -z "$COMMIT" ]]; then
    echo "Error: Unable to retrieve commit hash."
    exit 1
fi

if [[ -z "$DATE" ]]; then
    echo "Error: Unable to retrieve date."
    exit 1
fi

if [[ "$OSTYPE" == "linux"* ]]; then
    GOOS=linux
    GOARCH=amd64
elif [[ "$OSTYPE" == "darwin"* ]]; then
    GOOS=darwin
    GOARCH=arm64
elif [[ "$OSTYPE" == "msys" ]]; then
    GOOS=windows
    GOARCH=amd64
fi


go build -o ./bin/zzh \
    -ldflags "-w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -trimpath \
    -v \
    ./cmd/zzh/{main,terminal}.go