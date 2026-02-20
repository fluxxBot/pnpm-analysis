#!/bin/bash
set -euo pipefail

# Configuration — set these or export as environment variables
ARTIFACTORY_URL="${ARTIFACTORY_URL:-https://your-company.jfrog.io/artifactory}"
ARTIFACTORY_TOKEN="${ARTIFACTORY_TOKEN:-your-token-here}"

CONCURRENCY="${CONCURRENCY:-15}"
BATCH_SIZE="${BATCH_SIZE:-30}"
PROJECT_DIR="${PROJECT_DIR:-..}"
# Modes: aql (AQL bulk only), storage (Storage API only), both (run both and compare)
MODE="${MODE:-both}"
SAMPLE="${SAMPLE:-20}"

echo "=== pnpm Checksum Collection Benchmark ==="
echo "Artifactory: $ARTIFACTORY_URL"
echo "Project Dir: $PROJECT_DIR"
echo "Concurrency: $CONCURRENCY"
echo "AQL Batch Size: $BATCH_SIZE"
echo "Mode: $MODE"
echo ""

go run main.go \
  --artifactory-url "$ARTIFACTORY_URL" \
  --artifactory-token "$ARTIFACTORY_TOKEN" \
  --project-dir "$PROJECT_DIR" \
  --concurrency "$CONCURRENCY" \
  --batch-size "$BATCH_SIZE" \
  --mode "$MODE" \
  --sample "$SAMPLE"
