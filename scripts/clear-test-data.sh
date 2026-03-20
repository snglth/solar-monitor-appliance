#!/usr/bin/env bash
# Delete all time series from VictoriaMetrics.
#
# Usage:
#   ./scripts/clear-test-data.sh                       # default: http://10.44.0.1:8428
#   ./scripts/clear-test-data.sh http://localhost:8428  # custom endpoint
set -euo pipefail

ENDPOINT="${1:-http://10.44.0.1:8428}"

echo "Deleting all series from ${ENDPOINT} ..."
curl -s --data-urlencode 'match[]={__name__=~".+"}' "${ENDPOINT}/api/v1/admin/tsdb/delete_series"
echo "Done."
