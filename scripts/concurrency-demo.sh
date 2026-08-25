#!/usr/bin/env bash
set -euo pipefail

client_bin="${CLIENT_BIN:-/tmp/queuemaxxing-client}"
queue_url="${QUEUE_URL:-http://localhost:8080}"
queue_name="concurrency-demo-$(date +%s)-$$"
result_dir="$(mktemp -d "${TMPDIR:-/tmp}/queuemaxxing-results.XXXXXX")"

cleanup() {
	rm -rf "${result_dir}"
}
trap cleanup EXIT INT TERM

if [[ ! -x "${client_bin}" ]]; then
	printf 'Client binary not found at %s. Run the setup commands first.\n' "${client_bin}"
	exit 1
fi

if ! QUEUE_URL="${queue_url}" "${client_bin}" health >/dev/null 2>&1; then
	printf 'The queue server is not running at %s. Start it in Terminal 1 first.\n' "${queue_url}"
	exit 1
fi

printf '\nCONCURRENCY DEMO\n'
printf '%s\n' '----------------'
printf 'Queue: %s\n\n' "${queue_name}"

printf '1. Create an empty FIFO queue\n'
printf '   $ QUEUE_URL=%s %s create %s --ordering fifo\n' \
	"${queue_url}" "${client_bin}" "${queue_name}"
create_response="${result_dir}/create.txt"
QUEUE_URL="${queue_url}" "${client_bin}" create "${queue_name}" \
	--ordering fifo >"${create_response}"
sed 's/^/   /' "${create_response}"
printf '\n'

printf '2. Enqueue exactly one message\n'
printf '   $ QUEUE_URL=%s %s enqueue %s --body "only available message"\n' \
	"${queue_url}" "${client_bin}" "${queue_name}"
enqueue_response="${result_dir}/enqueue.txt"
QUEUE_URL="${queue_url}" "${client_bin}" enqueue "${queue_name}" \
	--body "only available message" >"${enqueue_response}"
sed 's/^/   /' "${enqueue_response}"
printf '\n'

printf '3. Dequeue concurrently with two consumers\n'
result_one="${result_dir}/consumer-1.txt"
result_two="${result_dir}/consumer-2.txt"

printf '   $ QUEUE_URL=%s %s dequeue %s  # Consumer 1\n' \
	"${queue_url}" "${client_bin}" "${queue_name}"
printf '   $ QUEUE_URL=%s %s dequeue %s  # Consumer 2\n\n' \
	"${queue_url}" "${client_bin}" "${queue_name}"

QUEUE_URL="${queue_url}" "${client_bin}" dequeue "${queue_name}" >"${result_one}" &
consumer_one_pid=$!
QUEUE_URL="${queue_url}" "${client_bin}" dequeue "${queue_name}" >"${result_two}" &
consumer_two_pid=$!

wait "${consumer_one_pid}"
wait "${consumer_two_pid}"

received=0
for consumer in 1 2; do
	if [[ "${consumer}" -eq 1 ]]; then
		result_file="${result_one}"
	else
		result_file="${result_two}"
	fi

	printf '   Consumer %s response:\n' "${consumer}"
	sed 's/^/      /' "${result_file}"

	if grep -q '"body": "only available message"' "${result_file}"; then
		received=$((received + 1))
	elif ! grep -q '^queue is empty$' "${result_file}"; then
		printf '   Consumer %s: unexpected response\n' "${consumer}"
	fi
	printf '\n'
done

if [[ "${received}" -eq 1 ]]; then
	printf 'PASS: both consumers ran concurrently, but the message was delivered once.\n'
else
	printf 'FAIL: expected one successful dequeue, got %s.\n' "${received}"
	exit 1
fi
