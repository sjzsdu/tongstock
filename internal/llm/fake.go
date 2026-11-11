package llm

import (
	"context"
	"io"
	"sync"
)

// completeResult is one scripted outcome of a Complete call.
type completeResult struct {
	resp Response
	err  error
}

// streamScript is one scripted outcome of a Stream call.
type streamScript struct {
	events   []Event
	terminal error // returned by Recv after events are exhausted (optional)
	openErr  error // returned directly by Stream (optional)
}

// FakeProvider is a scriptable Provider for contract tests. It records every
// Request it receives and replays pre-scripted responses, events and errors.
// Construct it with NewFakeProvider.
type FakeProvider struct {
	mu       sync.Mutex
	name     string
	results  []completeResult
	streams  []streamScript
	requests []Request
}

// NewFakeProvider returns an empty scripted provider. An empty name defaults
// to "fake".
func NewFakeProvider(name string) *FakeProvider {
	if name == "" {
		name = "fake"
	}
	return &FakeProvider{name: name}
}

// Name implements Provider.
func (f *FakeProvider) Name() string { return f.name }

// QueueComplete scripts the next Complete call to return (resp, err). Calls
// are served FIFO; when err is non-nil, resp is ignored. With an empty queue,
// Complete returns a zero Response and a nil error.
func (f *FakeProvider) QueueComplete(resp Response, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = append(f.results, completeResult{resp: resp, err: err})
}

// QueueStream scripts the next Stream call to emit events in order and then
// return terminalErr from Recv (io.EOF when nil). The events slice is copied.
func (f *FakeProvider) QueueStream(events []Event, terminalErr error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.streams = append(f.streams, streamScript{
		events:   append([]Event(nil), events...),
		terminal: terminalErr,
	})
}

// QueueStreamError scripts the next Stream call to fail at creation time.
func (f *FakeProvider) QueueStreamError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.streams = append(f.streams, streamScript{openErr: err})
}

// Requests returns a snapshot of every Request received so far, in call order.
func (f *FakeProvider) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.requests...)
}

// LastRequest returns the most recently received Request.
func (f *FakeProvider) LastRequest() (Request, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return Request{}, false
	}
	return f.requests[len(f.requests)-1], true
}

// Complete implements Provider.
func (f *FakeProvider) Complete(ctx context.Context, req Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if len(f.results) == 0 {
		return Response{}, nil
	}
	r := f.results[0]
	f.results = f.results[1:]
	if r.err != nil {
		return Response{}, r.err
	}
	return r.resp, nil
}

// Stream implements Provider.
func (f *FakeProvider) Stream(ctx context.Context, req Request) (Stream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	var script streamScript
	if len(f.streams) > 0 {
		script = f.streams[0]
		f.streams = f.streams[1:]
	}
	if script.openErr != nil {
		return nil, script.openErr
	}
	return &fakeStream{
		ctx:      ctx,
		events:   script.events,
		terminal: script.terminal,
	}, nil
}

// fakeStream is the Stream returned by FakeProvider. It binds the context
// passed to Stream so cancellation is observable through Recv.
type fakeStream struct {
	ctx      context.Context
	events   []Event
	terminal error

	mu           sync.Mutex
	next         int
	closed       bool
	terminalUsed bool
}

// Recv implements Stream. Cancellation takes priority: once the context is
// done, Recv returns the context error rather than any buffered event or
// io.EOF. After the scripted events (and optional terminal error) are
// consumed, Recv returns io.EOF. A closed stream also returns io.EOF.
func (s *fakeStream) Recv() (Event, error) {
	if err := s.ctx.Err(); err != nil {
		return Event{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Event{}, io.EOF
	}
	if s.next < len(s.events) {
		ev := s.events[s.next]
		s.next++
		return ev, nil
	}
	if s.terminal != nil && !s.terminalUsed {
		s.terminalUsed = true
		return Event{}, s.terminal
	}
	return Event{}, io.EOF
}

// Close implements Stream and is idempotent.
func (s *fakeStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

var _ Provider = (*FakeProvider)(nil)
var _ Stream = (*fakeStream)(nil)
