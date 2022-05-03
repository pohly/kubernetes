#!/usr/bin/env bash

# Copyright 2021 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script checks the coding style for the Go language files using
# golangci-lint. Which checks are enabled depends on command line flags. The
# default is a minimal set of checks that all existing code passes without
# issues.

usage () {
    cat <<EOF >&2
Usage: $0 [-r <revision>|-a] [-s] [-g <github action file>] [packages]"
   -r <revision>: only report issues in code added since that revision
   -a: automatically select the common base of origin/master and HEAD
       as revision
   -s: select a strict configuration for new code
   -g <github action file>: also write results with --out-format=github-actions
       to a separate file
   [packages]: check specific packages or directories instead of everything
EOF
    exit 1
}

set -o errexit
set -o nounset
set -o pipefail

KUBE_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
source "${KUBE_ROOT}/hack/lib/init.sh"
source "${KUBE_ROOT}/hack/lib/util.sh"

invocation=(./hack/verify-golangci-lint.sh "$@")
golangci=(golangci-lint run)
golangci_config="${KUBE_ROOT}/hack/golangci.yaml"
base=
strict=
githubactions=
while getopts "r:asg:" o; do
    case "${o}" in
        r)
            base="${OPTARG}"
            if [ ! "$base" ]; then
                echo "ERROR: -c needs a non-empty parameter" >&2
                echo >&2
                usage
            fi
            ;;
        a)
            base="$(git merge-base origin/master HEAD)"
            ;;
        s)
            golangci_config="${KUBE_ROOT}/hack/golangci-strict.yaml"
            strict=1
            ;;
        g)
            githubactions="${OPTARG}"
            ;;
        *)
            usage
            ;;
    esac
done
shift $((OPTIND-1))
golangci+=(--config="${golangci_config}")

if [ "$base" ]; then
    # Must be a something that git can resolve to a commit.
    # "git rev-parse --verify" checks that and prints a detailed
    # error.
    base="$(git rev-parse --verify "$base")"
    golangci+=(--new --new-from-rev="$base")
fi

kube::golang::verify_go_version

# Ensure that we find the binaries we build before anything else.
export GOBIN="${KUBE_OUTPUT_BINPATH}"
PATH="${GOBIN}:${PATH}"

# Explicitly opt into go modules, even though we're inside a GOPATH directory
export GO111MODULE=on

# Install golangci-lint
echo "installing golangci-lint and logcheck plugin from hack/tools into ${GOBIN}"
pushd "${KUBE_ROOT}/hack/tools" >/dev/null
  go install github.com/golangci/golangci-lint/cmd/golangci-lint
  go build -o "${GOBIN}/logcheck.so" -buildmode=plugin sigs.k8s.io/logtools/logcheck/plugin
popd >/dev/null

cd "${KUBE_ROOT}"

# The logcheck plugin currently has to be configured via env variables
# (https://github.com/golangci/golangci-lint/issues/1512).
#
# Remember to clean the golangci-lint cache when changing
# the configuration and running this script multiple times,
# otherwise golangci-lint will report stale results:
# _output/local/bin/golangci-lint cache clean
export LOGCHECK_CONFIG="${KUBE_ROOT}/hack/logcheck.conf"
echo "running ${golangci[*]}" >&2
res=0
run () {
    if [[ "$#" -gt 0 ]]; then
        "${golangci[@]}" "$@" >&2 || res=$?
    else
        "${golangci[@]}" ./... >&2 || res=$?
        for d in staging/src/k8s.io/*; do
            MODPATH="staging/src/k8s.io/$(basename "${d}")"
            echo "running golangci-lint for ${KUBE_ROOT}/${MODPATH}"
            pushd "${KUBE_ROOT}/${MODPATH}" >/dev/null
            "${golangci[@]}" --path-prefix "${MODPATH}" ./... >&2 || res=$?
            popd >/dev/null
        done
    fi
}
# First run with normal output.
run "$@"

# Then optionally do it again with github-actions as format.
# Because golangci-lint caches results, this is faster than the
# initial invocation.
if [ "$githubactions" ]; then
    golangci+=(--out-format=github-actions)
    run "$@" >"$githubactions" 2>&1
fi

# print a message based on the result
if [ "$res" -eq 0 ]; then
  echo 'Congratulations! All files are passing lint :-)'
else
  {
    echo
    echo "Please review the above warnings. You can test via \"${invocation[*]}\""
    echo 'If the above warnings do not make sense, you can exempt this warning with a comment'
    echo ' (if your reviewer is okay with it).'
    if [ "$strict" ]; then
        echo 'The more strict .golangci-strict.yaml was used. If you feel that this warns about issues'
        echo 'that should be ignored by default, then please discuss with your reviewer and propose'
        echo 'a change for .golangci-strict.yaml as part of your PR.'
    fi
    echo 'In general please prefer to fix the error, we have already disabled specific lints'
    echo ' that the project chooses to ignore.'
    echo 'See: https://golangci-lint.run/usage/false-positives/'
    echo
  } >&2
  exit 1
fi

# preserve the result
exit "$res"
