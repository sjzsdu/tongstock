package tdx

import (
	"context"
	"errors"
	"sync"
)

// Executor abstracts over a single *Client or a Pool. Every operation receives
// a borrowed Client and must return it (or signal replacement) via the provided
// callbacks. The abstraction keeps concurrency and borrow/put semantics inside
// the TDX infrastructure layer so that handlers/services never touch the
// connection mutex directly.
type Executor interface {
	Do(fn func(c *Client) error) error
	DoContext(ctx context.Context, fn func(c *Client) error) error
	Close() error
	// Len returns the number of live idle connections. For single-client
	// executors this is either 0 or 1.
	Len() int
}

// ExecutorStatus is a minimal snapshot used for diagnostics.
type ExecutorStatus struct {
	Size int  `json:"size"`
	Idle int  `json:"idle"`
	Open bool `json:"open"`
}

// NewExecutor returns a Pool-backed Executor when size > 1, otherwise a
// single-client executor. The constructor always calls dial exactly once in
// the single case and `size` times in the pool case; on any dial failure all
// previously created connections are closed before returning the error.
func NewExecutor(dial func() (*Client, error), size int) (Executor, error) {
	if size <= 1 {
		c, err := dial()
		if err != nil {
			return nil, err
		}
		return newSingleExecutor(c), nil
	}
	p, err := newPool(dial, size)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// singleExecutor wraps one *Client and provides the same Executor interface.
type singleExecutor struct {
	mu         sync.Mutex
	client     *Client
	permit     chan struct{}
	done       chan struct{}
	closedDone chan struct{}
	closeErr   error
	closed     bool
}

func newSingleExecutor(client *Client) *singleExecutor {
	permit := make(chan struct{}, 1)
	permit <- struct{}{}
	return &singleExecutor{
		client:     client,
		permit:     permit,
		done:       make(chan struct{}),
		closedDone: make(chan struct{}),
	}
}

func (s *singleExecutor) Do(fn func(c *Client) error) error {
	return s.DoContext(context.Background(), fn)
}

func (s *singleExecutor) DoContext(ctx context.Context, fn func(c *Client) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return errors.New("executor closed")
	case <-s.permit:
	}
	defer func() { s.permit <- struct{}{} }()

	s.mu.Lock()
	if s.closed || s.client == nil {
		s.mu.Unlock()
		return errors.New("executor closed")
	}
	c := s.client
	s.mu.Unlock()
	return fn(c)
}

func (s *singleExecutor) Close() error {
	s.mu.Lock()
	if s.closed {
		done := s.closedDone
		s.mu.Unlock()
		<-done
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.closeErr
	}
	s.closed = true
	close(s.done)
	s.mu.Unlock()

	// Wait for the in-flight operation, if any, before closing the connection.
	<-s.permit
	s.mu.Lock()
	if s.client == nil {
		close(s.closedDone)
		s.mu.Unlock()
		return nil
	}
	err := s.client.Close()
	s.client = nil
	s.closeErr = err
	close(s.closedDone)
	s.mu.Unlock()
	return err
}

func (s *singleExecutor) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.client == nil {
		return 0
	}
	return len(s.permit)
}

func (s *singleExecutor) Status() ExecutorStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	idle := 0
	if !s.closed && s.client != nil {
		idle = len(s.permit)
	}
	return ExecutorStatus{Size: 1, Idle: idle, Open: !s.closed && s.client != nil}
}

// Pool is a fixed-size connection pool with borrow/put semantics.
type Pool struct {
	idle       chan *Client
	permits    chan struct{}
	done       chan struct{}
	mu         sync.Mutex
	wg         sync.WaitGroup
	closed     bool
	closedDone chan struct{}
	closeErr   error
	dial       func() (*Client, error)
	size       int
}

// newPool creates a pool of the given size. If any dial attempt fails, all
// connections created so far are closed and the error is returned.
func newPool(dial func() (*Client, error), size int) (*Pool, error) {
	if size <= 0 {
		size = 1
	}
	p := &Pool{
		idle:       make(chan *Client, size),
		permits:    make(chan struct{}, size),
		done:       make(chan struct{}),
		closedDone: make(chan struct{}),
		dial:       dial,
		size:       size,
	}
	created := make([]*Client, 0, size)
	for i := 0; i < size; i++ {
		c, err := dial()
		if err != nil {
			for _, cc := range created {
				_ = cc.Close()
			}
			return nil, err
		}
		created = append(created, c)
		p.idle <- c
		p.permits <- struct{}{}
	}
	return p, nil
}

// NewPool is kept for backwards compatibility. Prefer NewExecutor.
func NewPool(dial func() (*Client, error), size int) (*Pool, error) {
	return newPool(dial, size)
}

// Get borrows a Client from the fixed-size pool.
func (p *Pool) Get() (*Client, error) {
	return p.GetContext(context.Background())
}

// GetContext borrows a Client, waiting for an existing slot when the pool is
// fully utilized. The wait can be canceled through ctx.
func (p *Pool) GetContext(ctx context.Context) (*Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("pool closed")
	}
	p.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.done:
		return nil, errors.New("pool closed")
	case <-p.permits:
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		p.permits <- struct{}{}
		return nil, errors.New("pool closed")
	}
	p.wg.Add(1)
	p.mu.Unlock()

	select {
	case c := <-p.idle:
		return c, nil
	default:
		c, err := p.dial()
		if err != nil {
			p.finishBorrow()
			return nil, err
		}
		return c, nil
	}
}

// Put returns a Client to the pool. If the pool is already at capacity or
// closed the client is closed instead (e.g. a dead/reconnecting client must
// not be pushed back).
func (p *Pool) Put(c *Client) {
	if c == nil {
		p.finishBorrow()
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		_ = c.Close()
		p.finishBorrow()
		return
	}
	select {
	case p.idle <- c:
	default:
		_ = c.Close()
	}
	p.mu.Unlock()
	p.finishBorrow()
}

func (p *Pool) discard(c *Client) {
	if c != nil {
		_ = c.Close()
	}
	p.finishBorrow()
}

func (p *Pool) finishBorrow() {
	p.wg.Done()
	p.permits <- struct{}{}
}

// Do borrows a Client, runs fn, and returns the Client to the pool. If fn
// returns an error the Client is closed (not returned) so that a damaged
// connection cannot poison the pool. Callers can also force replacement by
// returning a non-nil error from fn.
func (p *Pool) Do(fn func(c *Client) error) error {
	return p.DoContext(context.Background(), fn)
}

func (p *Pool) DoContext(ctx context.Context, fn func(c *Client) error) error {
	c, err := p.GetContext(ctx)
	if err != nil {
		return err
	}
	healthy := false
	defer func() {
		if healthy {
			p.Put(c)
		} else {
			// This also runs during panic unwinding, so a panicking callback
			// cannot leak a borrowed connection.
			p.discard(c)
		}
	}()

	err = fn(c)
	if err != nil {
		return err
	}
	healthy = true
	return nil
}

// Close drains the pool and closes every idle client. It is safe to call
// Close more than once.
func (p *Pool) Close() error {
	p.mu.Lock()
	if p.closed {
		done := p.closedDone
		p.mu.Unlock()
		<-done
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.closeErr
	}
	p.closed = true
	close(p.done)
	p.mu.Unlock()

	// GetContext increments wg while holding mu, so no Add can race with this
	// Wait after closed becomes visible.
	p.wg.Wait()

	var firstErr error
	for {
		select {
		case c := <-p.idle:
			if err := c.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		default:
			p.mu.Lock()
			p.closeErr = firstErr
			close(p.closedDone)
			p.mu.Unlock()
			return firstErr
		}
	}
}

// Len returns the number of currently idle connections in the pool.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0
	}
	return len(p.idle)
}

// Status returns a minimal diagnostic snapshot.
func (p *Pool) Status() ExecutorStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return ExecutorStatus{Size: p.size, Idle: len(p.idle), Open: !p.closed}
}
