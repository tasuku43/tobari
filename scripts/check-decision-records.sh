#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
decision_directory=${1:-docs/decisions}

if [[ ! -d $decision_directory ]]; then
  echo "ADR directory does not exist: $decision_directory" >&2
  exit 1
fi

temporary_file=$(mktemp)
cleanup() { rm -f -- "$temporary_file"; }
trap cleanup EXIT

for path in "$decision_directory"/*.md; do
  [[ -e $path ]] || continue
  name=${path##*/}
  if [[ ! $name =~ ^([0-9]{4})-[a-z0-9][a-z0-9-]*\.md$ ]]; then
    echo "ADR filename is invalid: $path" >&2
    exit 1
  fi
  number=${BASH_REMATCH[1]}
  heading=$(sed -n '1p' "$path")
  if [[ $heading != "# ADR $number: "* ]]; then
    echo "ADR heading does not match filename $number: $path" >&2
    exit 1
  fi
  printf '%s\t%s\n' "$number" "$path" >>"$temporary_file"
done

duplicate=$(cut -f1 "$temporary_file" | LC_ALL=C sort | uniq -d | sed -n '1p')
if [[ -n $duplicate ]]; then
  echo "duplicate ADR number $duplicate:" >&2
  awk -F '\t' -v number="$duplicate" '$1 == number { print "  " $2 }' "$temporary_file" >&2
  exit 1
fi

echo "decision record identity: OK"
