package nats

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeNATSServer speaks enough of the NATS text protocol over loopback TCP
// to exercise real *nats.Conn subscribe, message delivery, unsubscribe, and drain flows.
type fakeNATSServer struct {
	ln     net.Listener
	mu     sync.Mutex
	conn   net.Conn
	writer *bufio.Writer
	unsubs chan string
	subs   map[string]string // sid -> subject
	subCh  chan string
}

func startFakeNATSServer(t *testing.T) *fakeNATSServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := &fakeNATSServer{
		ln:     ln,
		unsubs: make(chan string, 16),
		subs:   make(map[string]string),
		subCh:  make(chan string, 16),
	}
	go s.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		s.mu.Lock()
		if s.conn != nil {
			_ = s.conn.Close()
		}
		s.mu.Unlock()
	})
	return s
}

func (s *fakeNATSServer) url() string {
	return "nats://" + s.ln.Addr().String()
}

func (s *fakeNATSServer) serve() {
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	w := bufio.NewWriter(conn)
	s.mu.Lock()
	s.conn = conn
	s.writer = w
	_, _ = w.WriteString(`INFO {"server_id":"FAKE","version":"2.10.0","proto":1,"max_payload":1048576,"headers":true}` + "\r\n")
	_ = w.Flush()
	s.mu.Unlock()

	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "PING"):
			s.writeLine("PONG")
		case strings.HasPrefix(trimmed, "SUB "):
			fields := strings.Fields(trimmed)
			if len(fields) >= 3 {
				subj := fields[1]
				sid := fields[len(fields)-1]
				s.mu.Lock()
				s.subs[sid] = subj
				s.mu.Unlock()
				select {
				case s.subCh <- sid:
				default:
				}
			}
		case strings.HasPrefix(trimmed, "UNSUB "):
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				select {
				case s.unsubs <- fields[1]:
				default:
				}
			}
		case strings.HasPrefix(trimmed, "PUB "):
			fields := strings.Fields(trimmed)
			if len(fields) >= 3 {
				var n int
				_, _ = fmt.Sscanf(fields[len(fields)-1], "%d", &n)
				if n > 0 {
					buf := make([]byte, n+2)
					_, _ = r.Read(buf)
				}
			}
		}
	}
}

func (s *fakeNATSServer) writeLine(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer == nil {
		return
	}
	_, _ = s.writer.WriteString(line + "\r\n")
	_ = s.writer.Flush()
}

func (s *fakeNATSServer) deliver(sid, subject string, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer == nil {
		return
	}
	_, _ = fmt.Fprintf(s.writer, "MSG %s %s %d\r\n%s\r\n", subject, sid, len(payload), payload)
	_ = s.writer.Flush()
}

// stubJetStream adapts QueueSubscribe to a real core nats.Conn QueueSubscribe so tests can
// verify *nats.Subscription lifecycle without a JetStream server.
type stubJetStream struct {
	nats.JetStreamContext
	nc *nats.Conn
}

func (j *stubJetStream) QueueSubscribe(subj, queue string, cb nats.MsgHandler, _ ...nats.SubOpt) (*nats.Subscription, error) {
	return j.nc.QueueSubscribe(subj, queue, cb)
}

func TestSubscriberUnsubscribesOnContextCancel(t *testing.T) {
	srv := startFakeNATSServer(t)
	nc, err := nats.Connect(srv.url())
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	sub, err := NewSubscriber("workers", nc, &stubJetStream{nc: nc}, &nats.ConsumerInfo{
		Stream: "orders",
		Name:   "workers",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	err = sub.Subscribe(ctx, "orders.created", func(context.Context, pubsub.Message, map[string]string) error {
		return nil
	})
	require.NoError(t, err)

	var sid string
	select {
	case sid = <-srv.subCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SUB command")
	}
	assert.Equal(t, 1, nc.NumSubscriptions())

	cancel()

	select {
	case unsubSID := <-srv.unsubs:
		assert.Equal(t, sid, unsubSID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for UNSUB after context cancellation")
	}
	assert.Eventually(t, func() bool {
		return nc.NumSubscriptions() == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestSubscriberSubscribeRejectsCanceledContext(t *testing.T) {
	srv := startFakeNATSServer(t)
	nc, err := nats.Connect(srv.url())
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	sub, err := NewSubscriber("workers", nc, &stubJetStream{nc: nc}, &nats.ConsumerInfo{
		Stream: "orders",
		Name:   "workers",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = sub.Subscribe(ctx, "orders.created", func(context.Context, pubsub.Message, map[string]string) error {
		return nil
	})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, nc.NumSubscriptions())
}

func TestSubscriberCloseWaitsForDrainAndPreservesClosedHandler(t *testing.T) {
	srv := startFakeNATSServer(t)

	closedHandlerCalled := make(chan struct{}, 1)
	nc, err := nats.Connect(
		srv.url(),
		nats.DrainTimeout(2*time.Second),
		nats.ClosedHandler(func(*nats.Conn) {
			closedHandlerCalled <- struct{}{}
		}),
	)
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	sub, err := NewSubscriber("workers", nc, &stubJetStream{nc: nc}, &nats.ConsumerInfo{
		Stream: "orders",
		Name:   "workers",
	})
	require.NoError(t, err)

	inHandler := make(chan struct{})
	releaseHandler := make(chan struct{})
	handlerDone := make(chan struct{})
	err = sub.Subscribe(context.Background(), "orders.created", func(context.Context, pubsub.Message, map[string]string) error {
		close(inHandler)
		<-releaseHandler
		close(handlerDone)
		return nil
	})
	require.NoError(t, err)

	var sid string
	select {
	case sid = <-srv.subCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SUB command")
	}

	srv.deliver(sid, "orders.created", []byte("in-flight"))
	select {
	case <-inHandler:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler to start")
	}

	closeErrCh := make(chan error, 1)
	go func() {
		closeErrCh <- sub.Close()
	}()

	// Close must not return while the in-flight handler is still running.
	select {
	case err := <-closeErrCh:
		t.Fatalf("Close returned before in-flight handler completed (err=%v)", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseHandler)
	<-handlerDone

	select {
	case err := <-closeErrCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Close to finish draining")
	}

	assert.True(t, nc.IsClosed(), "connection must be closed when Close returns")
	select {
	case <-closedHandlerCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("caller's ClosedHandler was not invoked")
	}
}

func TestSubscriberCloseReturnsDrainTimeout(t *testing.T) {
	srv := startFakeNATSServer(t)

	nc, err := nats.Connect(srv.url(), nats.DrainTimeout(100*time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	sub, err := NewSubscriber("workers", nc, &stubJetStream{nc: nc}, &nats.ConsumerInfo{
		Stream: "orders",
		Name:   "workers",
	})
	require.NoError(t, err)

	inHandler := make(chan struct{})
	releaseHandler := make(chan struct{})
	t.Cleanup(func() { close(releaseHandler) })
	err = sub.Subscribe(context.Background(), "orders.created", func(context.Context, pubsub.Message, map[string]string) error {
		close(inHandler)
		<-releaseHandler
		return nil
	})
	require.NoError(t, err)

	var sid string
	select {
	case sid = <-srv.subCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SUB command")
	}

	srv.deliver(sid, "orders.created", []byte("stuck"))
	<-inHandler

	err = sub.Close()
	assert.ErrorIs(t, err, nats.ErrDrainTimeout)
	assert.True(t, nc.IsClosed())
}
