package vinyl

import (
	"bytes"
	"errors"
	"io"
	"sync"
)

// Contents models the three states a vinyl file's `contents` property can be
// in, exactly as in the JS implementation:
//
//	Buffer  -> file.isBuffer()   (contents buffered in memory)
//	*Stream -> file.isStream()   (contents are a lazily-opened reader)
//	nil     -> file.isNull()     (src() was called with read:false, or the
//	                              entry is a directory / symlink)
//
// Only the two concrete types below satisfy the interface. The sealed method
// returns a discriminant rather than nothing so that IsBuffer and IsStream
// dispatch through one place; adding a third state means adding a kind here.
type Contents interface {
	contents() kind
}

type kind int

const (
	kindBuffer kind = iota
	kindStream
)

// Buffer holds file contents in memory. It is the default produced by src().
type Buffer []byte

func (Buffer) contents() kind { return kindBuffer }

// Stream holds file contents as a lazily-opened, re-openable reader.
//
// The JS implementation relies on `lazystream` so that (a) file descriptors
// are not opened until something actually reads, and (b) dest() can "reset"
// the contents stream after writing so downstream consumers can read it again
// (documented under "Returns" in docs/api/dest.md). Stream reproduces both
// properties: Open is only invoked on the first Read, and Reset discards the
// current reader so the next Read re-invokes Open.
type Stream struct {
	// open produces a fresh reader over the same logical contents.
	open func() (io.ReadCloser, error)

	mu     sync.Mutex
	rc     io.ReadCloser
	closed bool
}

func (*Stream) contents() kind { return kindStream }

// NewStream returns a Stream that calls open lazily on first read and again
// after every Reset. open must be safe to call multiple times.
func NewStream(open func() (io.ReadCloser, error)) *Stream {
	return &Stream{open: open}
}

// NewStreamFromBytes returns a Stream backed by an in-memory buffer. Useful
// for plugins that synthesise contents but must present a streaming file.
func NewStreamFromBytes(b []byte) *Stream {
	return NewStream(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(b)), nil
	})
}

// ErrStreamClosed is returned when reading from a Stream that has been closed
// and not subsequently reset.
var ErrStreamClosed = errors.New("vinyl: contents stream is closed")

// Read implements io.Reader, opening the underlying reader on first use.
func (s *Stream) Read(p []byte) (int, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return 0, ErrStreamClosed
	}
	if s.rc == nil {
		rc, err := s.open()
		if err != nil {
			s.mu.Unlock()
			return 0, err
		}
		s.rc = rc
	}
	rc := s.rc
	s.mu.Unlock()
	return rc.Read(p)
}

// Close releases the underlying reader. A closed Stream can be revived with
// Reset.
func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.rc == nil {
		return nil
	}
	err := s.rc.Close()
	s.rc = nil
	return err
}

// Reset closes any open reader and arms the Stream to be read from the
// beginning again. This is what dest() calls after writing a streaming file so
// that the re-emitted vinyl object remains consumable downstream.
func (s *Stream) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if s.rc != nil {
		err = s.rc.Close()
		s.rc = nil
	}
	s.closed = false
	return err
}

// Bytes drains the stream into memory and resets it, so the Stream stays
// usable afterwards. Plugins that need random access use this to convert a
// streaming file into a buffered one.
func (s *Stream) Bytes() ([]byte, error) {
	if err := s.Reset(); err != nil {
		return nil, err
	}
	b, err := io.ReadAll(s)
	if resetErr := s.Reset(); err == nil {
		err = resetErr
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}
