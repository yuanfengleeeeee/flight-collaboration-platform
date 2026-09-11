#!/bin/sh
set -eu

child_pids=""

start_child() {
    "$@" &
    child_pids="$child_pids $!"
}

stop_children() {
    for pid in $child_pids; do
        kill "$pid" 2>/dev/null || true
    done
    wait 2>/dev/null || true
}

trap stop_children INT TERM EXIT

start_child /app/bin/core-api -config /app/config.yaml
start_child /app/bin/edge-api -config /app/config.yaml
start_child /app/bin/worker -config /app/config.yaml

nginx -g 'daemon off;' &
nginx_pid=$!
all_pids="$child_pids $nginx_pid"

while :; do
    for pid in $all_pids; do
        if ! kill -0 "$pid" 2>/dev/null; then
            exit 1
        fi
    done
    sleep 1
done
