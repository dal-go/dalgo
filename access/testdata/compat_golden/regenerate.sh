#!/bin/sh
# Writes access/testdata/compat_golden/decisions.golden.
#
# The golden holds the decisions that policies with no table rule give over every
# way a query or a key can name a source. It must come from code that has no table
# rules, so this script refuses to run unless the non-test Go sources of the
# access and dal packages are those of the commit it is given: the commit before
# the first commit that added table rules.
#
#   access/testdata/compat_golden/regenerate.sh <commit>
#
# The matrix is access/compat_golden_test.go (TestCompatGolden). It uses no name
# that table rules added, so it compiles and runs on that commit.
set -eu

parent="${1:?usage: regenerate.sh <commit that has no table rules>}"
root="$(git rev-parse --show-toplevel)"
cd "$root"

if ! git diff --quiet "$parent" -- access dal ':(exclude)*_test.go' ':(exclude)access/testdata'; then
  echo "refusing: the Go sources of access and dal differ from $parent" >&2
  exit 1
fi
if [ -n "$(git ls-files --others --exclude-standard -- access dal ':(exclude)*_test.go' ':(exclude)access/testdata')" ]; then
  echo "refusing: untracked sources in access or dal" >&2
  exit 1
fi

if command -v wb >/dev/null 2>&1; then
  DALGO_COMPAT_GOLDEN_PARENT="$parent" wb run -- go test ./access -run '^TestCompatGolden$' -count=1 -v
else
  DALGO_COMPAT_GOLDEN_PARENT="$parent" go test ./access -run '^TestCompatGolden$' -count=1 -v
fi
