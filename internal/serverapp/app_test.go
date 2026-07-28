package serverapp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/server"
	"github.com/sjzsdu/tongstock/pkg/tdx"
)

type fakeExecutor struct {
	open atomic.Bool
}

func TestUnknownAPIRouteReturnsStableErrorInsteadOfSPA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(server.RequestID())
	setupStaticRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	var envelope server.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "not_found" || envelope.Error.RequestID == "" {
		t.Fatalf("envelope = %+v", envelope)
	}
}

type blockingListener struct {
	done chan struct{}
	once sync.Once
}

func newBlockingListener() *blockingListener {
	return &blockingListener{done: make(chan struct{})}
}

func (l *blockingListener) Accept() (net.Conn, error) {
	<-l.done
	return nil, net.ErrClosed
}

func (l *blockingListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *blockingListener) Addr() net.Addr { return fakeAddr("memory") }

type fakeAddr string

func (a fakeAddr) Network() string { return "memory" }
func (a fakeAddr) String() string  { return string(a) }

func newFakeExecutor() *fakeExecutor {
	executor := &fakeExecutor{}
	executor.open.Store(true)
	return executor
}

func (e *fakeExecutor) Do(fn func(*tdx.Client) error) error {
	return e.DoContext(context.Background(), fn)
}

func (e *fakeExecutor) DoContext(ctx context.Context, fn func(*tdx.Client) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(nil)
}

func (e *fakeExecutor) Close() error {
	e.open.Store(false)
	return nil
}

func (e *fakeExecutor) Len() int {
	if e.open.Load() {
		return 1
	}
	return 0
}

func (e *fakeExecutor) Status() tdx.ExecutorStatus {
	return tdx.ExecutorStatus{Size: 1, Idle: e.Len(), Open: e.open.Load()}
}

func TestAppRepeatedRunAndIdempotentShutdown(t *testing.T) {
	for iteration := 0; iteration < 3; iteration++ {
		executor := newFakeExecutor()
		cfg := config.DefaultConfig()
		cfg.Database.DSN = filepath.Join(t.TempDir(), "app.db")
		cfg.Server.BindAddress = "127.0.0.1"
		cfg.Agent.Enabled = false
		listener := newBlockingListener()

		app, err := NewApp(cfg, Options{
			ExecutorFactory: func() (tdx.Executor, error) { return executor, nil },
			Listen: func(_, _ string) (net.Listener, error) {
				return listener, nil
			},
			SkipProcessFile: true,
		})
		if err != nil {
			t.Fatalf("iteration %d NewApp() error = %v", iteration, err)
		}
		if app.httpServer.ReadHeaderTimeout != readHeaderTimeout ||
			app.httpServer.ReadTimeout != readTimeout ||
			app.httpServer.IdleTimeout != idleTimeout ||
			app.httpServer.WriteTimeout != writeTimeout {
			t.Fatalf("unexpected HTTP timeout configuration: %+v", app.httpServer)
		}

		runCtx, cancelRun := context.WithCancel(context.Background())
		runDone := make(chan error, 1)
		go func() { runDone <- app.Run(runCtx) }()

		var address string
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case err := <-runDone:
				t.Fatalf("App stopped before listening: %v", err)
			default:
			}
			app.runMu.Lock()
			if app.listener != nil {
				address = app.listener.Addr().String()
			}
			app.runMu.Unlock()
			if address != "" {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if address == "" {
			t.Fatal("App did not start listening")
		}

		diagnostics := app.Diagnostics(context.Background())
		if diagnostics.Status == "unavailable" {
			t.Fatalf("diagnostics unexpectedly unavailable: %+v", diagnostics)
		}

		cancelRun()
		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run() did not stop after cancellation")
		}
		if err := app.Shutdown(context.Background()); err != nil {
			t.Fatalf("second Shutdown() error = %v", err)
		}
		if executor.open.Load() {
			t.Fatal("executor remained open after shutdown")
		}
	}
}

func TestNewAppFailureRollsBackConstructedResources(t *testing.T) {
	executor := newFakeExecutor()
	cfg := config.DefaultConfig()
	cfg.Database.DSN = filepath.Join(t.TempDir(), "rollback.db")
	cfg.Server.BindAddress = "0.0.0.0"
	cfg.Server.AccessToken = ""

	app, err := NewApp(cfg, Options{
		ExecutorFactory: func() (tdx.Executor, error) { return executor, nil },
		SkipProcessFile: true,
	})
	if err == nil || app != nil {
		t.Fatalf("NewApp() = (%v, %v), want bind security failure", app, err)
	}
	if executor.open.Load() {
		t.Fatal("executor remained open after construction rollback")
	}
}

func TestAppListenerFailureCanBeShutDownIdempotently(t *testing.T) {
	executor := newFakeExecutor()
	cfg := config.DefaultConfig()
	cfg.Database.DSN = filepath.Join(t.TempDir(), "listen-failure.db")
	listenErr := errors.New("address unavailable")
	app, err := NewApp(cfg, Options{
		ExecutorFactory: func() (tdx.Executor, error) { return executor, nil },
		Listen: func(_, _ string) (net.Listener, error) {
			return nil, listenErr
		},
		SkipProcessFile: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Run(context.Background()); !errors.Is(err, listenErr) {
		t.Fatalf("Run() error = %v", err)
	}
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if executor.open.Load() {
		t.Fatal("executor remained open after listener failure")
	}
}
