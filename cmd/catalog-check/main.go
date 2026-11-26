package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/scmbr/smart-search/internal/catalog"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("catalog-check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("file", "", "YML file path, or - for stdin; compression is detected by signature")
	recordLimit := flags.Int64("max-record-bytes", 1<<20, "Maximum bytes per XML record/token")
	documentLimit := flags.Int64("max-document-bytes", 1<<30, "Maximum decoded document bytes")
	issueLimit := flags.Int("max-issues", 100, "Maximum retained diagnostic details")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *path == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: catalog-check -file PATH [-max-issues 100]")
		return 2
	}
	input := stdin
	if *path != "-" {
		file, err := os.Open(*path)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		defer file.Close() // Read-only input; no buffered writes to flush.
		input = file
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	report, readErr := catalog.Read(ctx, input, nil, catalog.Options{
		MaxRecordBytes: *recordLimit, MaxDocumentBytes: *documentLimit, MaxIssues: *issueLimit,
	})
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	result := struct {
		catalog.Report
		ElapsedMS           int64  `json:"elapsed_ms"`
		TotalAllocatedBytes uint64 `json:"total_allocated_bytes"`
		HeapAllocatedBytes  uint64 `json:"heap_allocated_bytes_after_check"`
	}{report, elapsed.Milliseconds(), after.TotalAlloc - before.TotalAlloc, after.HeapAlloc}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(stderr, "write report:", err)
		return 1
	}
	if readErr != nil {
		fmt.Fprintln(stderr, "catalog check:", readErr)
		return 1
	}
	if !report.CanActivate {
		return 2
	}
	return 0
}
