#!/bin/bash
# Copyright 2026 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

usage() {
    echo "Usage: $0 [-r RUNS] [-o OUTPUT_DIR] [-t TEST_NAME] [-x TEST_NAME] [-h]"
    echo ""
    echo "Options:"
    echo "  -r RUNS        Number of runs per test (default: 5)"
    echo "  -o OUTPUT_DIR  Output directory for results (default: test-results/qa)"
    echo "  -t TEST_NAME   Run only this test suite (e.g. 01-smoke). Can be repeated."
    echo "  -x TEST_NAME   Exclude this test suite. Can be repeated. Mutually exclusive with -t."
    echo "  -h             Show this help"
    echo ""
    echo "Examples:"
    echo "  $0                                      # All tests, 5 runs each"
    echo "  $0 -r 3                                 # All tests, 3 runs each"
    echo "  $0 -t 03-ipoe-local -r 10               # Single test, 10 runs"
    echo "  $0 -t 01-smoke -t 02-smoke-ha           # Two tests, 5 runs each"
    echo "  $0 -x 09-cgnat-ipoe-det -x 11-cgnat-pppoe-det  # All tests except these two"
    exit 0
}

RUNS=5
QA_DIR=""
FILTER_TESTS=()
EXCLUDE_TESTS=()

while getopts "r:o:t:x:h" opt; do
    case $opt in
        r) RUNS="$OPTARG" ;;
        o) QA_DIR="$OPTARG" ;;
        t) FILTER_TESTS+=("$OPTARG") ;;
        x) EXCLUDE_TESTS+=("$OPTARG") ;;
        h) usage ;;
        *) usage ;;
    esac
done

if [ ${#FILTER_TESTS[@]} -gt 0 ] && [ ${#EXCLUDE_TESTS[@]} -gt 0 ]; then
    echo "Error: -t and -x are mutually exclusive"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_DIR"

if [ -z "$QA_DIR" ]; then
    QA_DIR="$REPO_DIR/test-results/qa"
fi

is_excluded() {
    local name="$1"
    for excluded in "${EXCLUDE_TESTS[@]}"; do
        if [ "$name" = "$excluded" ]; then
            return 0
        fi
    done
    return 1
}

TESTS=()
if [ ${#FILTER_TESTS[@]} -gt 0 ]; then
    for name in "${FILTER_TESTS[@]}"; do
        robot="tests/${name}/${name}.robot"
        if [ -f "$robot" ]; then
            TESTS+=("$name")
        else
            echo "Error: test suite not found: $robot"
            exit 1
        fi
    done
else
    for excluded in "${EXCLUDE_TESTS[@]}"; do
        if [ ! -f "tests/${excluded}/${excluded}.robot" ]; then
            echo "Error: excluded test suite not found: tests/${excluded}/${excluded}.robot"
            exit 1
        fi
    done
    for dir in tests/*/; do
        name=$(basename "$dir")
        robot="$dir/${name}.robot"
        if [ -f "$robot" ] && ! is_excluded "$name"; then
            TESTS+=("$name")
        fi
    done
fi

if [ ${#TESTS[@]} -eq 0 ]; then
    echo "No test suites found"
    exit 1
fi

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULTS_DIR="$QA_DIR/$TIMESTAMP"
SUMMARY="$RESULTS_DIR/summary.txt"

mkdir -p "$RESULTS_DIR"

echo "QA Test Run: $TIMESTAMP" | tee "$SUMMARY"
echo "Runs per test: $RUNS" | tee -a "$SUMMARY"
echo "Tests: ${TESTS[*]}" | tee -a "$SUMMARY"
echo "Results: $RESULTS_DIR" | tee -a "$SUMMARY"
echo "========================================" | tee -a "$SUMMARY"

total_pass=0
total_fail=0

for test in "${TESTS[@]}"; do
    pass=0
    fail=0
    echo "" | tee -a "$SUMMARY"
    echo "--- $test ---" | tee -a "$SUMMARY"

    for i in $(seq 1 "$RUNS"); do
        log_file="$RESULTS_DIR/${test}-run${i}.log"
        echo -n "  Run $i/$RUNS: "

        if output=$(make robot-test suite="$test" 2>&1); then
            result=$(echo "$output" | grep "tests," | tail -1)
            echo "PASS ($result)" | tee -a "$SUMMARY"
            pass=$((pass + 1))
            total_pass=$((total_pass + 1))
        else
            result=$(echo "$output" | grep "tests," | tail -1)
            echo "FAIL ($result)" | tee -a "$SUMMARY"
            fail=$((fail + 1))
            total_fail=$((total_fail + 1))
        fi

        echo "$output" > "$log_file"
    done

    echo "  Result: $pass/$RUNS passed" | tee -a "$SUMMARY"
done

echo "" | tee -a "$SUMMARY"
echo "========================================" | tee -a "$SUMMARY"
echo "TOTAL: $total_pass passed, $total_fail failed out of $((total_pass + total_fail))" | tee -a "$SUMMARY"
echo "Done: $(date)" | tee -a "$SUMMARY"