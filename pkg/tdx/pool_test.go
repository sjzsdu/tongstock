package tdx

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type trackedConn struct {
	closed atomic.Bool
}

func (c *trackedConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *trackedConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *trackedConn) Close() error                     { c.closed.Store(true); return nil }
func (c *trackedConn) LocalAddr() net.Addr              { return testAddr("local") }
func (c *trackedConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (c *trackedConn) SetDeadline(time.Time) error      { return nil }
func (c *trackedConn) SetReadDeadline(time.Time) error  { return nil }
func (c *trackedConn) SetWriteDeadline(time.Time) error { return nil }

type testAddr string

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return string(a) }

func newTrackedClient() (*Client, *trackedConn) {
	conn := &trackedConn{}
	return &Client{conn: conn, done: make(chan struct{})}, conn
}

func TestClientCloseIsIdempotentAndStopsHeartbeat(t *testing.T) {
	client, conn := newTrackedClient()

	if err := client.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if !conn.closed.Load() {
		t.Fatal("Close() did not close the connection")
	}
	select {
	case <-client.done:
	default:
		t.Fatal("Close() did not signal the heartbeat goroutine")
	}
}

func TestNewPoolClosesPartialInitialization(t *testing.T) {
	var conns []*trackedConn
	attempts := 0
	pool, err := newPool(func() (*Client, error) {
		attempts++
		if attempts == 3 {
			return nil, errors.New("dial failed")
		}
		client, conn := newTrackedClient()
		conns = append(conns, conn)
		return client, nil
	}, 3)
	if err == nil {
		t.Fatal("newPool() error = nil, want dial failure")
	}
	if pool != nil {
		t.Fatal("newPool() returned a pool after partial initialization failure")
	}
	for i, conn := range conns {
		if !conn.closed.Load() {
			t.Errorf("connection %d was not closed", i)
		}
	}
}

func TestPoolWaitIsCancelableAndCapacityIsFixed(t *testing.T) {
	dialCount := 0
	pool, err := newPool(func() (*Client, error) {
		dialCount++
		if dialCount > 1 {
			t.Errorf("pool dialed beyond its configured capacity")
		}
		client, _ := newTrackedClient()
		return client, nil
	}, 1)
	if err != nil {
		t.Fatalf("newPool() error = %v", err)
	}

	borrowed, err := pool.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := pool.GetContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetContext() error = %v, want deadline exceeded", err)
	}
	pool.Put(borrowed)
	if err := pool.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestPoolPanicDiscardsClientAndReleasesSlot(t *testing.T) {
	var mu sync.Mutex
	var conns []*trackedConn
	pool, err := newPool(func() (*Client, error) {
		client, conn := newTrackedClient()
		mu.Lock()
		conns = append(conns, conn)
		mu.Unlock()
		return client, nil
	}, 1)
	if err != nil {
		t.Fatalf("newPool() error = %v", err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("Do() did not propagate callback panic")
			}
		}()
		_ = pool.Do(func(*Client) error {
			panic("boom")
		})
	}()

	if !conns[0].closed.Load() {
		t.Fatal("panicking callback did not close the borrowed client")
	}
	if err := pool.Do(func(*Client) error { return nil }); err != nil {
		t.Fatalf("Do() after panic error = %v", err)
	}
	if len(conns) != 2 {
		t.Fatalf("dial count = %d, want replacement connection", len(conns))
	}
	if err := pool.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestPoolCloseWaitsForBorrowedClient(t *testing.T) {
	pool, err := newPool(func() (*Client, error) {
		client, _ := newTrackedClient()
		return client, nil
	}, 1)
	if err != nil {
		t.Fatalf("newPool() error = %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	doDone := make(chan error, 1)
	go func() {
		doDone <- pool.Do(func(*Client) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	closeDone := make(chan error, 1)
	go func() { closeDone <- pool.Close() }()
	secondCloseDone := make(chan error, 1)
	go func() { secondCloseDone <- pool.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("Close() returned before callback completed: %v", err)
	case err := <-secondCloseDone:
		t.Fatalf("concurrent Close() returned before callback completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	if err := <-doDone; err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := <-secondCloseDone; err != nil {
		t.Fatalf("concurrent Close() error = %v", err)
	}
}

func TestSingleExecutorSerializesAndCloseWaits(t *testing.T) {
	client, conn := newTrackedClient()
	exec := newSingleExecutor(client)
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- exec.Do(func(*Client) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- exec.Do(func(*Client) error {
			close(secondEntered)
			return nil
		})
	}()
	select {
	case <-secondEntered:
		t.Fatal("single executor allowed concurrent callbacks")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Do() error = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second Do() error = %v", err)
	}
	if err := exec.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !conn.closed.Load() {
		t.Fatal("Close() did not close the single client")
	}
}

func TestServiceClosesOwnedLegacyExecutor(t *testing.T) {
	client, conn := newTrackedClient()
	svc := &Service{
		executor:     newSingleExecutor(client),
		ownsExecutor: true,
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("Service.Close() error = %v", err)
	}
	if !conn.closed.Load() {
		t.Fatal("Service.Close() did not close its owned executor")
	}
}
