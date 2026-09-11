package bach

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	errFirst  = errors.New("first")
	errSecond = errors.New("second")
)

// record returns a task that appends its name to log, so ordering assertions
// read as a plain slice comparison.
func record(log *[]string, mu *sync.Mutex, name string) TaskFunc {
	return func(context.Context) error {
		mu.Lock()
		*log = append(*log, name)
		mu.Unlock()
		return nil
	}
}

func fail(err error) TaskFunc {
	return func(context.Context) error { return err }
}

func TestSeriesRunsInOrder(t *testing.T) {
	var mu sync.Mutex
	var log []string

	err := Series(
		record(&log, &mu, "a"),
		record(&log, &mu, "b"),
		record(&log, &mu, "c"),
	)(context.Background())
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	if got := len(log); got != 3 || log[0] != "a" || log[1] != "b" || log[2] != "c" {
		t.Fatalf("log = %v, want [a b c]", log)
	}
}

func TestSeriesStopsAtFirstError(t *testing.T) {
	var ran atomic.Bool

	err := Series(
		fail(errFirst),
		func(context.Context) error {
			ran.Store(true)
			return nil
		},
	)(context.Background())

	if !errors.Is(err, errFirst) {
		t.Fatalf("err = %v, want errFirst", err)
	}
	if ran.Load() {
		t.Fatal("the second task ran after the first failed")
	}
}

func TestSeriesWithNoTasks(t *testing.T) {
	if err := Series()(context.Background()); err != nil {
		t.Fatalf("Series(): %v", err)
	}
}

func TestParallelRunsConcurrently(t *testing.T) {
	var started sync.WaitGroup
	started.Add(2)
	release := make(chan struct{})

	block := func(context.Context) error {
		started.Done()
		<-release
		return nil
	}

	done := make(chan error, 1)
	go func() { done <- Parallel(block, block)(context.Background()) }()

	// Both tasks must reach their barrier before either is released, which is
	// only possible if they run at the same time.
	waited := make(chan struct{})
	go func() { started.Wait(); close(waited) }()

	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("tasks did not start concurrently")
	}
	close(release)

	if err := <-done; err != nil {
		t.Fatalf("Parallel: %v", err)
	}
}

// JavaScript's bach cannot stop a sibling that is already running. Go can, and
// the port takes that option deliberately: a failing build should not wait for
// work whose result is about to be thrown away.
func TestParallelCancelsSiblingsOnError(t *testing.T) {
	cancelled := make(chan struct{})

	slow := func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			close(cancelled)
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return errors.New("was not cancelled")
		}
	}

	err := Parallel(fail(errFirst), slow)(context.Background())
	if !errors.Is(err, errFirst) {
		t.Fatalf("err = %v, want errFirst", err)
	}

	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("the sibling was not cancelled")
	}
}

func TestParallelWithNoTasks(t *testing.T) {
	if err := Parallel()(context.Background()); err != nil {
		t.Fatalf("Parallel(): %v", err)
	}
}

func TestSettleSeriesRunsEveryTask(t *testing.T) {
	var mu sync.Mutex
	var log []string

	err := SettleSeries(
		record(&log, &mu, "a"),
		fail(errFirst),
		record(&log, &mu, "c"),
	)(context.Background())

	if !errors.Is(err, errFirst) {
		t.Fatalf("err = %v, want errFirst", err)
	}
	if len(log) != 2 || log[0] != "a" || log[1] != "c" {
		t.Fatalf("log = %v, want [a c] — the tasks after a failure must still run", log)
	}
}

func TestSettleSeriesAggregatesErrors(t *testing.T) {
	err := SettleSeries(fail(errFirst), fail(errSecond))(context.Background())
	if !errors.Is(err, errFirst) || !errors.Is(err, errSecond) {
		t.Fatalf("err = %v, want both failures joined", err)
	}
}

func TestSettleParallelRunsEveryTask(t *testing.T) {
	var count atomic.Int32
	tick := func(context.Context) error {
		count.Add(1)
		return nil
	}

	err := SettleParallel(tick, fail(errFirst), tick)(context.Background())
	if !errors.Is(err, errFirst) {
		t.Fatalf("err = %v, want errFirst", err)
	}
	if got := count.Load(); got != 2 {
		t.Fatalf("ran %d successful tasks, want 2", got)
	}
}

// Settle exists so that --continue can report every failure; cancelling
// siblings would defeat that.
func TestSettleParallelDoesNotCancelSiblings(t *testing.T) {
	finished := make(chan struct{})

	slow := func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
			close(finished)
			return nil
		}
	}

	if err := SettleParallel(fail(errFirst), slow)(context.Background()); !errors.Is(err, errFirst) {
		t.Fatalf("err = %v, want errFirst", err)
	}

	select {
	case <-finished:
	default:
		t.Fatal("the sibling was cancelled by the failure")
	}
}

func TestSeriesHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var ran atomic.Bool

	err := Series(
		func(context.Context) error {
			cancel()
			return nil
		},
		func(context.Context) error {
			ran.Store(true)
			return nil
		},
	)(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if ran.Load() {
		t.Fatal("a task ran after the context was cancelled")
	}
}
