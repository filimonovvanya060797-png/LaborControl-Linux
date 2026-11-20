#!/usr/bin/env bash
# Real ELF process smoke test on GitHub-hosted Ubuntu, with completely
# synthetic state in an isolated XDG_DATA_HOME. Never touches user data.
set -euo pipefail

BIN="${1:?Usage: bash tests/smoke_linux.sh /path/to/laborcontrol-controller}"
TMP="$(mktemp -d)"
export XDG_DATA_HOME="$TMP/xdg"
PID=""

cleanup() {
  if [[ -n "$PID" ]]; then
    curl -sS -m 1 -X POST http://127.0.0.1:8779/api/shutdown >/dev/null 2>&1 || true
    kill "$PID" >/dev/null 2>&1 || true
    wait "$PID" >/dev/null 2>&1 || true
  fi
  if [[ "${1:-0}" != "0" ]]; then
    echo "Server launch log:" >&2
    cat "$TMP/server.log" >&2 || true
  fi
  rm -rf "$TMP"
}
trap 'cleanup "$?"' EXIT

start() {
  "$BIN" >"$TMP/server.log" 2>&1 &
  PID=$!
  for _ in $(seq 1 100); do
    if curl -fsS -m 1 http://127.0.0.1:8779/health >"$TMP/health.json" 2>/dev/null; then
      python3 - "$TMP/health.json" <<'PY'
import json,sys
data=json.load(open(sys.argv[1]))
assert data["ok"] and data["version"] == "R11-LINUX-CORE/0.1", data
PY
      return
    fi
    sleep 0.1
  done
  echo "R11 server not reachable after startup" >&2
  return 1
}

post() {
  curl -fsS -m 5 -X POST -H 'Content-Type: application/json' --data "$2" \
    "http://127.0.0.1:8779$1" > "$TMP/response.json"
  python3 - "$TMP/response.json" <<'PY'
import json,sys
data=json.load(open(sys.argv[1]))
assert data.get("ok"), data
PY
}

start
curl -fsS -m 5 http://127.0.0.1:8779/r11 >"$TMP/page.html"
grep -q "Рабочая система" "$TMP/page.html"
grep -q "Перенос базы из Windows R11" "$TMP/page.html"

post "/api/employee/save" '{"tab_no":"CI-SYN-001","full_name":"Synthetic driver","collection_point":"CI TEST","schedule_type":"A"}'
post "/api/day/save" '{"TabNo":"CI-SYN-001","Date":"2026-11-20","Plan":"1/8","PlanHours":8,"Fact":"","FactHours":0}'
post "/api/day/quick-save" '{"TabNo":"CI-SYN-001","Date":"2026-11-20","Plan":"1/10","PlanHours":10}'
post "/api/backup" '{}'

curl -fsS -m 5 http://127.0.0.1:8779/api/state >"$TMP/state.json"
python3 - "$TMP/state.json" <<'PY'
import json,sys
state=json.load(open(sys.argv[1]))
assert len(state["data"]["employees"]) == 1
assert len(state["data"]["days"]) == 1
day=state["data"]["days"][0]
assert day["plan"]=="1/10" and day["plan_hours"]==10, day
PY

backups=("$XDG_DATA_HOME/LaborControl/backups/"*.json)
if (("${#backups[@]}" < 2)); then
  echo "Expected automatic backup files after edits" >&2
  exit 1
fi

curl -fsS -m 5 -X POST http://127.0.0.1:8779/api/shutdown >"$TMP/shutdown.json"
for _ in $(seq 1 100); do
  if ! kill -0 "$PID" 2>/dev/null; then break; fi
  sleep 0.1
done
if kill -0 "$PID" 2>/dev/null; then
  echo "Process did not shut down" >&2
  exit 1
fi
wait "$PID" || true
PID=""

start
curl -fsS -m 5 http://127.0.0.1:8779/api/state >"$TMP/restarted.json"
python3 - "$TMP/restarted.json" <<'PY'
import json,sys
state=json.load(open(sys.argv[1]))
assert state["data"]["days"][0]["plan"] == "1/10", state
assert state["data"]["days"][0]["plan_hours"] == 10, state
print("PASS: R11 Linux ELF startup, original UI, synthetic edits, backups and restart persistence")
PY
