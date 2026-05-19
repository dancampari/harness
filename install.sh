#!/usr/bin/env bash
# Build script for harness. Run from the project root.
set -euo pipefail

VERSION="${VERSION:-dev}"
DEST="${DEST:-$HOME/.local/bin/harness}"

echo "→ Resolving dependencies..."
go mod tidy

echo "→ Running gofmt check..."
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
  echo "  ✗ Unformatted files:"
  echo "$unformatted"
  exit 1
fi
echo "  ✓ All files formatted"

echo "→ Running go vet..."
go vet ./...
echo "  ✓ vet passed"

echo "→ Building binary..."
mkdir -p "$(dirname "$DEST")"
go build -ldflags "-X main.version=$VERSION" -o "$DEST" .
echo "  ✓ Built: $DEST"

if command -v "$DEST" >/dev/null 2>&1; then
  echo
  echo "Try it:"
  echo "  harness --version"
  echo "  harness init"
fi
