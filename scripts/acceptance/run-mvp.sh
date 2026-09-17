#!/usr/bin/env bash
set -u

artifact_dir=".artifacts"
cleanup_status="$artifact_dir/cleanup-status.json"
mkdir -p "$artifact_dir"
rm -f "$cleanup_status"
rm -f "$artifact_dir/acceptance-result.json" "$artifact_dir/acceptance-result.html" "$artifact_dir/acceptance-playwright.json"
rm -rf playwright-report test-results

if [[ -z "${KPI_TEST_HR_PASSWORD:-}" ]]; then
  echo "[ENV] missing required variable KPI_TEST_HR_PASSWORD" >&2
  exit 2
fi

base_url="${KPI_BASE_URL:-http://localhost}"
export KPI_TEST_TEMP_PASSWORD="${KPI_TEST_TEMP_PASSWORD:-Kpi-Acceptance-${RANDOM}-$(date +%s)}"
required_services=(backend frontend nginx-gateway)
running_services="$(docker compose ps --status running --services 2>/dev/null || true)"
for service in "${required_services[@]}"; do
  if ! grep -qx "$service" <<<"$running_services"; then
    echo "[ENV] Compose service is not running: $service" >&2
    exit 2
  fi
done

ready=false
for _ in {1..30}; do
  if curl --silent --show-error --fail "$base_url/health" | grep -q '"status":"OK"'; then
    ready=true
    break
  fi
  sleep 2
done
if [[ "$ready" != true ]]; then
  echo "[ENV] health endpoint did not become ready within 60 seconds" >&2
  exit 2
fi

if ! curl --silent --show-error --fail --output /dev/null "$base_url/auth/login"; then
  echo "[ENV] login page is unavailable through the gateway" >&2
  exit 2
fi

./node_modules/.bin/playwright test "$@"
test_status=$?

if [[ $test_status -eq 0 ]]; then
  node scripts/acceptance/scan-artifacts.mjs .artifacts test-results
  scan_status=$?
  if [[ $scan_status -ne 0 ]]; then
    test_status=1
  fi
fi

if [[ -f "$cleanup_status" ]] && grep -q '"status": "failed"' "$cleanup_status"; then
  exit 3
fi
if [[ $test_status -ne 0 ]]; then
  exit 1
fi
exit 0
