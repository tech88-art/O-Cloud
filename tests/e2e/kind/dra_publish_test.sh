#!/usr/bin/env bash
# tests/e2e/kind/dra_publish_test.sh — P4-T-104 kind smoke assertion.
#
# Verifies the npu-dra-driver simulator-first publisher (P4-T-005) is
# emitting ResourceSlices the live kube-apiserver accepts.
#
# Assertions (per docs/phase4-plan.md §3 P4-T-104):
#   1. At least 1 ResourceSlice present cluster-wide
#   2. At least 1 ResourceSlice has driver=="npu.ocloud.edge.example.com"
#   3. That slice carries at least 8 devices in spec.devices[]
#   4. At least one device has the npu.huawei.com/index attribute set
#
# Usage:
#   bash tests/e2e/kind/dra_publish_test.sh
#
# Designed for bash 4+, uses jq for JSON parsing. Aborts on the first
# unmet assertion via set -e + explicit `exit 1` paths so the workflow
# turns red with a useful stderr message.

set -euo pipefail

readonly EXPECTED_DRIVER="npu.ocloud.edge.example.com"
readonly EXPECTED_MIN_DEVICES=8
readonly EXPECTED_ATTR="npu.huawei.com/index"

if ! command -v kubectl >/dev/null 2>&1; then
  echo "::error::kubectl not on PATH" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "::error::jq not on PATH (install via apt-get install -y jq)" >&2
  exit 1
fi

echo "== fetch ResourceSlices =="
slices_json="$(kubectl get resourceslices -o json)"
echo "${slices_json}" | jq '{count: (.items | length), drivers: [.items[].spec.driver] | unique}'

# Assertion 1: at least 1 ResourceSlice present.
total="$(echo "${slices_json}" | jq '.items | length')"
if [[ "${total}" -lt 1 ]]; then
  echo "::error::expected >= 1 ResourceSlice, got ${total}" >&2
  exit 1
fi
echo "OK: ${total} ResourceSlice(s) present"

# Assertion 2: at least 1 slice has the expected driver.
matching_count="$(echo "${slices_json}" | jq --arg drv "${EXPECTED_DRIVER}" \
  '[.items[] | select(.spec.driver == $drv)] | length')"
if [[ "${matching_count}" -lt 1 ]]; then
  echo "::error::no ResourceSlice with driver=${EXPECTED_DRIVER}" >&2
  echo "::error::actual drivers: $(echo "${slices_json}" | jq -r '[.items[].spec.driver] | unique | join(",")')" >&2
  exit 1
fi
echo "OK: ${matching_count} slice(s) with driver=${EXPECTED_DRIVER}"

# Pick the first matching slice for device-level assertions.
slice_json="$(echo "${slices_json}" | jq --arg drv "${EXPECTED_DRIVER}" \
  '[.items[] | select(.spec.driver == $drv)][0]')"

# Assertion 3: at least 8 devices.
device_count="$(echo "${slice_json}" | jq '.spec.devices | length')"
if [[ "${device_count}" -lt "${EXPECTED_MIN_DEVICES}" ]]; then
  echo "::error::expected >= ${EXPECTED_MIN_DEVICES} devices in the matching slice; got ${device_count}" >&2
  echo "::error::slice name: $(echo "${slice_json}" | jq -r '.metadata.name')" >&2
  exit 1
fi
echo "OK: ${device_count} devices in first matching slice"

# Assertion 4: at least one device carries npu.huawei.com/index.
# v1beta1.Device.Basic.Attributes is a map; check any device has the key.
has_index_attr="$(echo "${slice_json}" | jq --arg attr "${EXPECTED_ATTR}" \
  '[.spec.devices[] | select(.basic.attributes[$attr] != null)] | length')"
if [[ "${has_index_attr}" -lt 1 ]]; then
  echo "::error::no device carries the ${EXPECTED_ATTR} attribute" >&2
  echo "::error::sample device: $(echo "${slice_json}" | jq -r '.spec.devices[0] | tojson')" >&2
  exit 1
fi
echo "OK: ${has_index_attr} device(s) carry ${EXPECTED_ATTR}"

echo "== dra_publish_test PASS =="
