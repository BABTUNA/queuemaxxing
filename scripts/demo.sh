#!/usr/bin/env bash
set -euo pipefail

fifo_queue="demo_fifo_$$"
lifo_queue="demo_lifo_$$"
client=(go run ./cmd/client)

echo "Creating FIFO queue: $fifo_queue"
"${client[@]}" create "$fifo_queue" --ordering fifo

echo "Enqueueing FIFO messages with priority"
"${client[@]}" enqueue "$fifo_queue" --body "first low priority" --priority 1
"${client[@]}" enqueue "$fifo_queue" --body "high priority" --priority 10
"${client[@]}" enqueue "$fifo_queue" --body "second low priority" --priority 1

echo "Expected: high priority, then FIFO order"
"${client[@]}" dequeue "$fifo_queue"
"${client[@]}" dequeue "$fifo_queue"
"${client[@]}" dequeue "$fifo_queue"

echo "Creating LIFO queue: $lifo_queue"
"${client[@]}" create "$lifo_queue" --ordering lifo
"${client[@]}" enqueue "$lifo_queue" --body "A"
"${client[@]}" enqueue "$lifo_queue" --body "B"

echo "Expected: B, then A"
"${client[@]}" dequeue "$lifo_queue"
"${client[@]}" dequeue "$lifo_queue"

echo "Enqueueing a message delayed by two seconds"
"${client[@]}" enqueue "$fifo_queue" --body "delayed message" --delay 2
"${client[@]}" dequeue "$fifo_queue"
sleep 2
"${client[@]}" dequeue "$fifo_queue"
