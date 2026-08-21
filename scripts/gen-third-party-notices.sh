#!/usr/bin/env bash
# Regenerate THIRD_PARTY_NOTICES.md from the modules linked into the released
# binaries. Run via `make notices` after changing dependencies.
#
# The license identifier for each module is recorded in scripts/licenses.tsv and
# is verified by hand — automatic classifiers routinely confuse BSD-2, BSD-3 and
# ISC. When a new module appears, read its LICENSE and add a row.
set -euo pipefail
cd "$(dirname "$0")/.."

MAP=scripts/licenses.tsv
OUT=THIRD_PARTY_NOTICES.md

# Modules linked into the goreleaser targets (darwin + linux).
linked=$(for os in darwin linux; do
  GOOS=$os GOARCH=amd64 go list -deps ./cmd/jira |
    GOOS=$os GOARCH=amd64 xargs go list -f '{{if .Module}}{{.Module.Path}}{{end}}'
done | grep -v '^$' | grep -v '^github.com/endgame-build/jira-cli$' | sort -u)

# Fail loudly when the linked set and the hand-verified map drift apart.
mapped=$(cut -f1 "$MAP" | sort -u)
if ! diff <(echo "$linked") <(echo "$mapped") >/dev/null; then
  echo "ERROR: $MAP is out of sync with the linked module set." >&2
  diff <(echo "$linked") <(echo "$mapped") >&2 || true
  echo "Read the LICENSE of each module marked '>' or '<' and update $MAP." >&2
  exit 1
fi

cat > "$OUT" <<'HDR'
# Third-Party Notices

jira-cli is licensed under the Apache License 2.0 (see `LICENSE`). The released
binaries statically link the Go modules below. Each module stays under its own
license; every license text is reproduced in full further down this file.

Two modules — `hashicorp/go-retryablehttp` and `hashicorp/go-cleanhttp` — are
covered by the Mozilla Public License 2.0. jira-cli uses both unmodified, so
MPL-2.0 §3.3 permits distributing the combined work under Apache-2.0 as long as
this notice tells you where to get their source. You can obtain it from the
upstream repositories listed below, or from the Go module proxy at
`https://proxy.golang.org/<module>/@v/<version>.zip`.

This list covers the modules linked into the darwin and linux builds produced by
`.goreleaser.yaml`. Build-time and test-only dependencies are excluded because
they are not distributed.

Regenerate with `make notices` after changing dependencies.

## Summary

| Module | Version | License |
|---|---|---|
HDR

while IFS=$'\t' read -r mod lic; do
  ver=$(go list -m -f '{{.Version}}' "$mod")
  printf '| `%s` | %s | %s |\n' "$mod" "$ver" "$lic" >> "$OUT"
done < "$MAP"

printf '\n## Full license texts\n' >> "$OUT"
while IFS=$'\t' read -r mod lic; do
  ver=$(go list -m -f '{{.Version}}' "$mod")
  dir=$(go list -m -f '{{.Dir}}' "$mod")
  file=$(find "$dir" -maxdepth 1 \( -iname 'LICENSE*' -o -iname 'COPYING*' \) | head -1)
  if [ -z "$file" ]; then
    echo "ERROR: no license file found for $mod in $dir" >&2
    exit 1
  fi
  { printf '\n### %s\n\nVersion %s — %s\n\n```text\n' "$mod" "$ver" "$lic"
    cat "$file"
    printf '```\n'; } >> "$OUT"
done < "$MAP"

echo "Wrote $OUT ($(wc -l < "$MAP" | tr -d ' ') modules, $(wc -c < "$OUT" | tr -d ' ') bytes)"
