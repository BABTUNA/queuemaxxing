package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	queueclient "github.com/BABTUNA/queuemaxxing/internal/client"
	"github.com/BABTUNA/queuemaxxing/internal/queue"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("a command is required")
	}

	client := queueclient.New(environment("QUEUE_URL", "http://localhost:8080"))
	switch args[0] {
	case "create":
		return runCreate(ctx, client, args[1:], stdout, stderr)
	case "get":
		return runGet(ctx, client, args[1:], stdout)
	case "enqueue":
		return runEnqueue(ctx, client, args[1:], stdout, stderr)
	case "dequeue":
		return runDequeue(ctx, client, args[1:], stdout)
	case "worker":
		return runWorker(ctx, client, args[1:], stdout, stderr)
	case "health":
		if len(args) != 1 {
			return errors.New("usage: queue-client health")
		}
		if err := client.Health(ctx); err != nil {
			return err
		}
		return printJSON(stdout, map[string]string{"status": "ok"})
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runCreate(ctx context.Context, client *queueclient.Client, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: queue-client create NAME --ordering fifo|lifo")
	}
	flags := flag.NewFlagSet("create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	ordering := flags.String("ordering", "", "equal-priority ordering: fifo or lifo")
	if err := flags.Parse(args[1:]); err != nil {
		return ignoreHelp(err)
	}
	if flags.NArg() != 0 || (*ordering != string(queue.FIFO) && *ordering != string(queue.LIFO)) {
		return errors.New("usage: queue-client create NAME --ordering fifo|lifo")
	}
	info, err := client.CreateQueue(ctx, args[0], queue.Ordering(*ordering))
	if err != nil {
		return err
	}
	return printJSON(stdout, info)
}

func runGet(ctx context.Context, client *queueclient.Client, args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: queue-client get NAME")
	}
	info, err := client.GetQueue(ctx, args[0])
	if err != nil {
		return err
	}
	return printJSON(stdout, info)
}

func runEnqueue(ctx context.Context, client *queueclient.Client, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: queue-client enqueue NAME --body TEXT [--priority N] [--delay SECONDS]")
	}
	flags := flag.NewFlagSet("enqueue", flag.ContinueOnError)
	flags.SetOutput(stderr)
	body := flags.String("body", "", "message body")
	priority := flags.Int64("priority", 0, "message priority")
	delay := flags.Int64("delay", 0, "delivery delay in seconds")
	if err := flags.Parse(args[1:]); err != nil {
		return ignoreHelp(err)
	}
	if flags.NArg() != 0 || *body == "" {
		return errors.New("usage: queue-client enqueue NAME --body TEXT [--priority N] [--delay SECONDS]")
	}
	if *priority < math.MinInt32 || *priority > math.MaxInt32 {
		return errors.New("priority must fit in a signed 32-bit integer")
	}
	if *delay < 0 || *delay > int64(queue.MaxDelay/time.Second) {
		return errors.New("delay must be between 0 and 900 seconds")
	}

	message, err := client.Enqueue(ctx, args[0], queueclient.EnqueueInput{
		Body:         *body,
		Priority:     int32(*priority),
		DelaySeconds: *delay,
	})
	if err != nil {
		return err
	}
	return printJSON(stdout, message)
}

func runDequeue(ctx context.Context, client *queueclient.Client, args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: queue-client dequeue NAME")
	}
	message, ok, err := client.Dequeue(ctx, args[0])
	if err != nil {
		return err
	}
	if !ok {
		_, err := fmt.Fprintln(stdout, "queue is empty")
		return err
	}
	return printJSON(stdout, message)
}

func runWorker(ctx context.Context, client *queueclient.Client, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: queue-client worker NAME [--interval 1s]")
	}
	flags := flag.NewFlagSet("worker", flag.ContinueOnError)
	flags.SetOutput(stderr)
	interval := flags.Duration("interval", time.Second, "empty-queue polling interval")
	if err := flags.Parse(args[1:]); err != nil {
		return ignoreHelp(err)
	}
	if flags.NArg() != 0 || *interval <= 0 {
		return errors.New("worker interval must be greater than zero")
	}

	for {
		message, ok, err := client.Dequeue(ctx, args[0])
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if ok {
			if err := printJSON(stdout, message); err != nil {
				return err
			}
			continue
		}

		timer := time.NewTimer(*interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func ignoreHelp(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

func printJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, `Usage: queue-client COMMAND [arguments]

Commands:
  create NAME --ordering fifo|lifo
  get NAME
  enqueue NAME --body TEXT [--priority N] [--delay SECONDS]
  dequeue NAME
  worker NAME [--interval 1s]
  health`)
}

func environment(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
