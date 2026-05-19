#!/usr/bin/env bash
# tests/e2e/kind/backend_metrics_test.sh — P4-T-106 backend /metrics smoke.
#
# Verifies the demo-backend Prometheus self-metrics endpoint (P4-T-007 +
# P4-T-008) is reachable and emits the three Ocloud counter families.
#
# Assertions:
#   1. GET /metrics returns 200 OK with Content-Type starting "text/plain"
#   2. Body contains at least one Go runtime collector (go_memstats_alloc_bytes)
#   3. Body contains at least one of the three Ocloud counter families:
#      - ocloud_backend_cache_eviction_total
#      - ocloud_backend_cache_hits_total
#      - ocloud_backend_dispatch_calls_total
#      (Note: counter families register with zero observed labelsets emit
#      no lines until at least one increment fires; the test warm-loops a
#      /api/v1 endpoint first so the dispatch counter has at least one
#      observation.)
#
# Usage:
#   bash tests/e2e/kind/backend_metrics_test.sh
#
# The kind-config maps NodePort 30080 to localhost:30080 so curl works
# against the host directly.

set -euo pipefail

readonly BACKEND_URL="${BACKEND_URL:-http://localhost:30080}"

echo "== warm dispatch counter via /api/v1/clusters =="
# Hit a real endpoint so Registry.SourceFor("clusters") fires and the
# dispatch counter has at least one observation (counter families need
# observed labelsets to emit lines).
for i in 1 2 3; do
  curl -fsS "${BACKEND_URL}/api/v1/clusters" >/dev/null || true
done

echo "== fetch /metrics =="
curl_out="$(curl -fsS -w '\n---HTTP_CODE: %{http_code}\n---CONTENT_TYPE: %{content_type}\n' \
  "${BACKEND_URL}/metrics")"
echo "${curl_out}" | tail -5

# Assertion 1: 200 + content-type.
if ! echo "${curl_out}" | grep -q '^---HTTP_CODE: 200$'; then
  echo "::error::GET ${BACKEND_URL}/metrics did not return 200" >&2
  echo "${curl_out}" | tail -5 >&2
  exit 1
fi
if ! echo "${curl_out}" | grep -q '^---CONTENT_TYPE: text/plain'; then
  echo "::error::Content-Type does not start text/plain" >&2
  echo "${curl_out}" | tail -5 >&2
  exit 1
fi
echo "OK: 200 + text/plain content-type"

# Assertion 2: standard Go runtime collector present.
body="$(echo "${curl_out}" | sed '/^---HTTP_CODE:/,$d')"
if ! echo "${body}" | grep -q '^go_memstats_'; then
  echo "::error::body missing go_memstats_* (Go default collectors not registered?)" >&2
  exit 1
fi
echo "OK: go_memstats_* present"

# Assertion 3: at least one Ocloud counter family present (after warm).
if ! echo "${body}" | grep -qE '^ocloud_backend_(cache_eviction_total|cache_hits_total|dispatch_calls_total)\{'; then
  echo "::error::body does not contain any ocloud_backend_* series after warm dispatch" >&2
  echo "::error::sample body tail:" >&2
  echo "${body}" | tail -30 >&2
  exit 1
fi
hits="$(echo "${body}" | grep -cE '^ocloud_backend_(cache_eviction_total|cache_hits_total|dispatch_calls_total)\{')"
echo "OK: ${hits} ocloud_backend_* series present"

echo "== backend_metrics_test PASS =="
