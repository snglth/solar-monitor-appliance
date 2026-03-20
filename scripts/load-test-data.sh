#!/usr/bin/env bash
# Load generated test data into VictoriaMetrics via the /write endpoint.
#
# Usage:
#   ./scripts/load-test-data.sh                       # default: http://10.44.0.1:8428
#   ./scripts/load-test-data.sh http://localhost:8428  # custom endpoint
set -euo pipefail

ENDPOINT="${1:-http://10.44.0.1:8428}"
BATCH_SIZE=5000
PARALLEL=8
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "Generating test data ..."
python3 "${SCRIPT_DIR}/generate-test-data.py" > "${tmpdir}/data.txt"

echo "Splitting into ${BATCH_SIZE}-line batches ..."
split -l "$BATCH_SIZE" "${tmpdir}/data.txt" "${tmpdir}/batch-"

batches="$(ls "${tmpdir}"/batch-* | wc -l | tr -d ' ')"
echo "Uploading ${batches} batches (${PARALLEL} parallel) to ${ENDPOINT}/write ..."

ls "${tmpdir}"/batch-* \
  | xargs -P "$PARALLEL" -I {} \
    curl -s -o /dev/null --data-binary @{} "${ENDPOINT}/write"

echo "Done. Sent ${batches} batches."
