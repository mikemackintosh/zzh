#!/bin/bash

LAST_TAG=$(git describe --tags --abbrev=0)
IFS='.' read -r -a VERSION_PARTS <<< "$LAST_TAG"
MAJOR=${VERSION_PARTS[0]}
MINOR=${VERSION_PARTS[1]}
PATCH=${VERSION_PARTS[2]}

# Increment the patch version
PATCH=$((PATCH + 1))

# Construct the new version
NEXT_VERSION="${MAJOR}.${MINOR}.${PATCH}"

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


go build -o ./bin/cli \
    -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -trimpath \
    -v \
    ./cmd/zzh/{main,terminal}.go