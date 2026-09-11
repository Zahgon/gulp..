package undertaker

import (
	"sync"
	"time"
)

// EventKind identifies a task lifecycle transition.
type EventKind int

const (
	// EventStart is emitted immediately before a task's function runs.
	EventStart EventKind = iota
	// EventStop is emitted after a task completes successfully.
	EventStop
	// EventError is emitted after a task fails.
	EventError
)

// String renders the kind using the same names the JS EventEmitter uses.
func (k EventKind) String() string {
	switch k {
	case EventStart:
		return "start"
	case EventStop:
		return "stop"
	case EventError:
		return "error"
	default:
		return "unknown"
	}
}

// Event describes a task lifecycle transition.
//
// In JS, Undertaker extends EventEmitter and emits 'start', 'stop' and 'error'
// objects; gulp-cli subscribes to them to print the familiar
// "Starting 'build'..." / "Finished 'build' after 2.3 ms" lines. This struct
// carries the same payload.
type Event struct {
	// UID identifies one execution, not one task. A task that runs three
	// times produces three uids, which is what lets a listener pair a stop
	// event with the start it belongs to even while tasks run concurrently.
	UID uint64
	// Name is the task's display label.
	Name string
	// Kind is the transition that occurred.
	Kind EventKind
	// Time is when the transition happened.
	Time time.Time
	// Duration is how long the task ran. Zero for EventStart.
	Duration time.Duration
	// Branch marks synthetic composition nodes (<series>, <parallel>).
	// gulp-cli suppresses log output for these, which is why real gulp output
	// never mentions them.
	Branch bool
	// Err is the failure, set only for EventError.
	Err error
}

// emitter is a goroutine-safe fan-out of Events to registered listeners.
type emitter struct {
	mu        sync.RWMutex
	listeners []func(Event)
}

// on registers a listener.
func (e *emitter) on(fn func(Event)) {
	if fn == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners = append(e.listeners, fn)
}

// emit delivers an event to every listener.
//
// Listeners are called synchronously so the CLI's log lines stay ordered
// relative to the work, but the slice is copied under the lock first so a
// listener that registers another listener cannot deadlock.
func (e *emitter) emit(ev Event) {
	e.mu.RLock()
	listeners := make([]func(Event), len(e.listeners))
	copy(listeners, e.listeners)
	e.mu.RUnlock()

	for _, fn := range listeners {
		fn(ev)
	}
}
