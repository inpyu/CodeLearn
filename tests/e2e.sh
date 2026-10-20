#!/usr/bin/env bash
# End-to-end acceptance scenario (SPEC 15.4), with no LLM involved.
#
# Creates a temporary repository, exercises export, import, every API endpoint
# and the staleness guarantee, then reports PASS or FAIL.
set -uo pipefail

BINARY="${1:-./codelearn}"
BINARY="$(cd "$(dirname "$BINARY")" && pwd)/$(basename "$BINARY")"
WORK="$(mktemp -d)"
export CODELEARN_CACHE_DIR="$WORK/cache"
PASS=0
FAIL=0

cleanup() {
  [ -n "${SERVER_PID:-}" ] && kill "$SERVER_PID" 2>/dev/null
  rm -rf "$WORK"
}
trap cleanup EXIT

check() { # check <description> <condition-as-command>
  if "${@:2}"; then
    echo "  PASS  $1"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $1"
    FAIL=$((FAIL + 1))
  fi
}

contains() { grep -q "$2" <<<"$1"; }

echo "== 1. temporary repository =="
REPO="$WORK/project"
mkdir -p "$REPO"
git -C "$REPO" init -q -b main
git -C "$REPO" config user.email test@example.com
git -C "$REPO" config user.name Test
cat > "$REPO/cache.go" <<'EOF'
package cache

func Get() int {
	return create()
}

func Clear() int {
	return 0
}
EOF
git -C "$REPO" add -A
git -C "$REPO" commit -qm "initial commit"

echo "== 2. modify files =="
cat > "$REPO/cache.go" <<'EOF'
package cache

func Get() int {
	return reuse()
}

func Clear() int {
	return 0
}
EOF
printf 'notes\n' > "$REPO/notes.txt"

echo "== 3. export =="
EXPORT_OUT="$("$BINARY" export "$REPO" --output "$WORK/changes.json" 2>&1)"
check "export succeeds" test $? -eq 0
check "export reports a snapshot id" contains "$EXPORT_OUT" "Snapshot: sha256:"
SNAPSHOT="$(python3 -c "import json;print(json.load(open('$WORK/changes.json'))['snapshot_id'])")"
CHANGE="$(python3 -c "
import json
d = json.load(open('$WORK/changes.json'))
print(next(c['id'] for c in d['changes'] if c.get('symbol') == 'Get'))")"
check "the changed function is a unit of its own" test -n "$CHANGE"
check "the unchanged function produced no unit" test "$(python3 -c "
import json
d = json.load(open('$WORK/changes.json'))
print(sum(1 for c in d['changes'] if c.get('symbol') == 'Clear'))")" -eq 0

echo "== 4. import an analysis =="
cat > "$WORK/analysis.json" <<EOF
{
  "schema_version": "1.0",
  "snapshot_id": "$SNAPSHOT",
  "generator": "e2e",
  "steps": [
    {
      "change_id": "$CHANGE",
      "title": "Cache reuse",
      "what": "Creation became reuse.",
      "how": "The existing object is returned.",
      "why": "Avoids paying construction cost per call."
    }
  ]
}
EOF
IMPORT_OUT="$("$BINARY" import "$WORK/analysis.json" "$REPO" 2>&1)"
check "import succeeds" contains "$IMPORT_OUT" "Imported 1 learning step(s)"
check "the analysis is stored outside the repository" test ! -e "$REPO/analysis.json"

echo "== 5. reject a stale analysis =="
sed "s/$SNAPSHOT/sha256:$(printf 'b%.0s' {1..64})/" "$WORK/analysis.json" > "$WORK/stale.json"
STALE_OUT="$("$BINARY" import "$WORK/stale.json" "$REPO" 2>&1)"
check "a snapshot mismatch is rejected" contains "$STALE_OUT" "snapshot_id"

echo "== 6. start the viewer =="
"$BINARY" "$REPO" --no-browser --port 0 > "$WORK/server.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 50); do
  URL="$(grep -o 'http://127.0.0.1:[0-9]*' "$WORK/server.log" | head -1)"
  [ -n "$URL" ] && curl -sf "$URL/api/repository" >/dev/null 2>&1 && break
  sleep 0.2
done
check "the viewer is serving" test -n "$URL"
check "the embedded page is served" test "$(curl -s -o /dev/null -w '%{http_code}' "$URL/")" = "200"

echo "== 7. API =="
check "repository endpoint" contains "$(curl -s "$URL/api/repository")" '"branch":"main"'
CHANGES="$(curl -s "$URL/api/changes")"
check "changes endpoint lists both files" test "$(python3 -c "
import json,sys;print(len(json.loads(sys.argv[1])['files']))" "$CHANGES")" -eq 2
DIFF="$(curl -s "$URL/api/files/diff?path=cache.go")"
check "file diff returns the code" contains "$DIFF" "reuse()"
check "file diff names the unit" contains "$DIFF" '"label":"Get()"'
check "an unknown path is refused" test "$(curl -s -o /dev/null -w '%{http_code}' "$URL/api/files/diff?path=../../etc/passwd")" = "404"
ANALYSIS="$(curl -s "$URL/api/analysis")"
check "analysis endpoint serves the step" contains "$ANALYSIS" '"title":"Cache reuse"'
check "the step carries its code location" contains "$ANALYSIS" '"file":"cache.go"'
check "refresh works" contains "$(curl -s -X POST "$URL/api/refresh")" '"snapshot_id":"sha256:'
check "a cross-origin write is refused" test "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Origin: https://evil.example' "$URL/api/refresh")" = "403"

echo "== 8. an unrelated edit keeps the explanation =="
printf 'more\n' > "$REPO/other.txt"
check "the step survives an unrelated change" contains "$(curl -s "$URL/api/analysis")" '"title":"Cache reuse"'

echo "== 9. editing the explained code withholds the explanation =="
cat > "$REPO/cache.go" <<'EOF'
package cache

func Get() int {
	return shared()
}

func Clear() int {
	return 0
}
EOF
AFTER="$(curl -s "$URL/api/analysis")"
check "the stale step is withheld" test "$(python3 -c "
import json,sys;print(len(json.loads(sys.argv[1])['steps']))" "$AFTER")" -eq 0
check "the drop is reported" contains "$AFTER" '"stale":true'

echo
echo "passed: $PASS   failed: $FAIL"
[ "$FAIL" -eq 0 ]
