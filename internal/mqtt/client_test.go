package mqtt

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/spydemon/mqtt2bdd/internal/logger"
)

// This file hand-writes fakes for mqtt.Client and mqtt.Token instead of using a
// third-party mocking library (e.g. gomock). Both are narrow interfaces — a
// handful of methods each — so implementing them by hand is both idiomatic Go
// and avoids a third-party dependency cost a mocking library would add, matching
// test-strategy-and-standards.md's "avoid over-mocking... mock at system
// boundaries" principle, applied at exactly the boundary (the Paho client)
// where this project's own code stops and a third-party library's begins.

// fakeToken is a minimal mqtt.Token: Done() is always already closed (this fake
// never simulates an in-flight operation), and Error() returns whatever the
// constructor was given.
type fakeToken struct {
	err  error
	done chan struct{}
}

func newFakeToken(err error) *fakeToken {
	done := make(chan struct{})
	close(done)
	return &fakeToken{err: err, done: done}
}

// newBlockingToken returns a fakeToken whose Done channel is never closed,
// simulating an in-flight operation that never completes on its own. Used to
// exercise a caller's own context-cancellation branch (Connect's
// case <-ctx.Done()), which no token that resolves immediately can reach.
func newBlockingToken() *fakeToken {
	return &fakeToken{done: make(chan struct{})}
}

func (t *fakeToken) Wait() bool                       { return true }
func (t *fakeToken) WaitTimeout(_ time.Duration) bool { return true }
func (t *fakeToken) Done() <-chan struct{}            { return t.done }
func (t *fakeToken) Error() error                     { return t.err }

// fakePahoClient implements mqtt.Client just enough to drive both this file's
// original reconnectLoop tests and this story's new Connect/Subscribe/
// IsConnected/Disconnect tests. Connect succeeds immediately by default; set
// connectErr or connectBlocks to script a failure or a never-completing attempt.
// Subscribe fails for subscribeFailures calls before succeeding, recording how
// many times it was invoked. disconnectCalls counts every Disconnect call, since
// Client's closeOnce only guards the shutdown channel close, not the underlying
// Paho call.
type fakePahoClient struct {
	subscribeFailures int
	subscribeCalls    int
	connected         bool  // returned by IsConnected(); defaults false, matching a fresh un-connected client
	connectErr        error // if non-nil, Connect() returns a token carrying this error
	connectBlocks     bool  // if true, Connect() returns a token whose Done() never closes
	disconnectCalls   int
}

func (f *fakePahoClient) IsConnected() bool      { return f.connected }
func (f *fakePahoClient) IsConnectionOpen() bool { return f.connected }
func (f *fakePahoClient) Connect() mqtt.Token {
	if f.connectBlocks {
		return newBlockingToken()
	}
	return newFakeToken(f.connectErr)
}
func (f *fakePahoClient) Disconnect(_ uint) { f.disconnectCalls++ }
func (f *fakePahoClient) Publish(_ string, _ byte, _ bool, _ interface{}) mqtt.Token {
	return newFakeToken(nil)
}

func (f *fakePahoClient) Subscribe(_ string, _ byte, _ mqtt.MessageHandler) mqtt.Token {
	f.subscribeCalls++
	if f.subscribeCalls <= f.subscribeFailures {
		return newFakeToken(errSubscribeFailed)
	}
	return newFakeToken(nil)
}

func (f *fakePahoClient) SubscribeMultiple(_ map[string]byte, _ mqtt.MessageHandler) mqtt.Token {
	return newFakeToken(nil)
}
func (f *fakePahoClient) Unsubscribe(_ ...string) mqtt.Token       { return newFakeToken(nil) }
func (f *fakePahoClient) AddRoute(_ string, _ mqtt.MessageHandler) {}
func (f *fakePahoClient) OptionsReader() mqtt.ClientOptionsReader {
	return mqtt.ClientOptionsReader{}
}

// errSubscribeFailed is the scripted error returned by fakePahoClient.Subscribe
// while it is still counting down subscribeFailures.
var errSubscribeFailed = &subscribeError{}

// subscribeError is a trivial error type so errSubscribeFailed needs no fmt import.
type subscribeError struct{}

func (*subscribeError) Error() string { return "simulated subscribe failure" }

// errConnectFailed is the scripted error returned by fakePahoClient.Connect via
// connectErr, exercised by TestConnect_Failure.
var errConnectFailed = &connectError{}

// connectError is a trivial error type so errConnectFailed needs no fmt import.
type connectError struct{}

func (*connectError) Error() string { return "simulated connect failure" }

// newTestClient constructs a Client wired to paho, bypassing NewClient (the same
// white-box pattern the existing reconnect tests use), so each new test below
// does not repeat the same struct literal.
func newTestClient(t *testing.T, paho mqtt.Client) *Client {
	t.Helper()
	return &Client{
		pahoClient: paho,
		logger:     logger.NewLogger("ERROR", io.Discard),
		shutdown:   make(chan struct{}),
	}
}

// TestConnect_Success exercises the happy path: a Paho Connect() that resolves
// immediately with no error.
func TestConnect_Success(t *testing.T) {
	t.Parallel()
	fake := &fakePahoClient{}
	c := newTestClient(t, fake)

	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() = %v, want nil", err)
	}
}

// TestConnect_Failure exercises the token.Error() branch: Paho resolves the
// token but with a non-nil error.
func TestConnect_Failure(t *testing.T) {
	t.Parallel()
	fake := &fakePahoClient{connectErr: errConnectFailed}
	c := newTestClient(t, fake)

	err := c.Connect(context.Background())
	if err == nil {
		t.Fatal("Connect() = nil, want an error")
	}
	if !errors.Is(err, errConnectFailed) {
		t.Errorf("Connect() error = %v, want it to wrap %v", err, errConnectFailed)
	}
}

// TestConnect_ContextCancelled exercises client.go's case <-ctx.Done() branch:
// an already-cancelled context against a token that never resolves on its own.
func TestConnect_ContextCancelled(t *testing.T) {
	t.Parallel()
	fake := &fakePahoClient{connectBlocks: true}
	c := newTestClient(t, fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Connect(ctx)
	if err == nil {
		t.Fatal("Connect() = nil, want an error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Connect() error = %v, want it to wrap context.Canceled", err)
	}
}

// TestSubscribe_Success exercises Subscribe's initial (not reconnect-driven) path.
func TestSubscribe_Success(t *testing.T) {
	t.Parallel()
	fake := &fakePahoClient{}
	c := newTestClient(t, fake)

	const topic = "sensors/#"
	handler := func(_ string, _ []byte) {}
	if err := c.Subscribe(topic, handler); err != nil {
		t.Fatalf("Subscribe() = %v, want nil", err)
	}
	if c.subscribedTopic != topic {
		t.Errorf("subscribedTopic = %q, want %q", c.subscribedTopic, topic)
	}
	if c.subscribedHandler == nil {
		t.Error("subscribedHandler = nil, want it recorded")
	}
}

// TestSubscribe_Failure asserts that Subscribe records subscribedTopic and
// subscribedHandler even when the attempt itself fails — client.go:120-148's
// "record first, attempt second" order, which is what lets a later reconnect
// retry the same subscription.
func TestSubscribe_Failure(t *testing.T) {
	t.Parallel()
	fake := &fakePahoClient{subscribeFailures: 1 << 30} // never succeeds within this test
	c := newTestClient(t, fake)

	const topic = "sensors/#"
	handler := func(_ string, _ []byte) {}
	err := c.Subscribe(topic, handler)
	if err == nil {
		t.Fatal("Subscribe() = nil, want an error")
	}
	if c.subscribedTopic != topic {
		t.Errorf("subscribedTopic = %q, want %q recorded even on failure", c.subscribedTopic, topic)
	}
	if c.subscribedHandler == nil {
		t.Error("subscribedHandler = nil, want it recorded even on failure")
	}
}

// TestIsConnected is a single non-tabled test rather than the trivial-getter
// exemption test-strategy-and-standards.md#what-not-to-test would otherwise
// allow: AC3 names "connection... logic" explicitly, and the delegation costs
// one line to prove.
func TestIsConnected(t *testing.T) {
	t.Parallel()
	fake := &fakePahoClient{connected: true}
	c := newTestClient(t, fake)

	if !c.IsConnected() {
		t.Error("IsConnected() = false, want true (delegates to pahoClient.IsConnected())")
	}
}

// TestDisconnect_ClosesShutdownExactlyOnce proves closeOnce guards the
// double-close(c.shutdown) case (no panic on a second Disconnect call), while
// confirming closeOnce does NOT guard the underlying Paho call: both calls must
// still reach c.pahoClient.Disconnect.
func TestDisconnect_ClosesShutdownExactlyOnce(t *testing.T) {
	t.Parallel()
	fake := &fakePahoClient{}
	c := newTestClient(t, fake)

	c.Disconnect()
	c.Disconnect() // must not panic on the second close(c.shutdown)

	if fake.disconnectCalls != 2 {
		t.Errorf("pahoClient.Disconnect called %d time(s), want 2", fake.disconnectCalls)
	}
}

// TestReconnectLoop_RetriesResubscribeUntilSuccess exercises AC12 / FIND-010: after
// a successful reconnect, a failing Subscribe must be retried until it succeeds,
// not abandoned after a single attempt.
func TestReconnectLoop_RetriesResubscribeUntilSuccess(t *testing.T) {
	// Shrunk for the duration of this test so the retry cadence does not incur a
	// real 10 s wait per failed attempt; restored afterward since this is a
	// package-level var shared with production code.
	original := defaultMQTTReconnectInterval
	defaultMQTTReconnectInterval = time.Millisecond
	t.Cleanup(func() { defaultMQTTReconnectInterval = original })

	fake := &fakePahoClient{subscribeFailures: 2}
	c := &Client{
		pahoClient:        fake,
		logger:            logger.NewLogger("ERROR", io.Discard),
		subscribedTopic:   "#",
		subscribedHandler: func(_ string, _ []byte) {},
		shutdown:          make(chan struct{}),
	}

	// reconnectLoop is called directly (unexported, same package), bypassing the
	// connection-lost handler that would normally launch it as a goroutine.
	c.reconnectLoop()

	if fake.subscribeCalls <= 1 {
		t.Fatalf("Subscribe called %d time(s), want more than 1 (proves a retry happened)", fake.subscribeCalls)
	}
	if fake.subscribeCalls != fake.subscribeFailures+1 {
		t.Fatalf("Subscribe called %d times, want exactly %d (fails %d times then succeeds)",
			fake.subscribeCalls, fake.subscribeFailures+1, fake.subscribeFailures)
	}
}

// TestReconnectLoop_ResubscribeStopsOnShutdown confirms the retry loop honours
// c.shutdown: a permanently-failing Subscribe must not retry forever once shutdown
// is signalled.
func TestReconnectLoop_ResubscribeStopsOnShutdown(t *testing.T) {
	original := defaultMQTTReconnectInterval
	defaultMQTTReconnectInterval = time.Millisecond
	t.Cleanup(func() { defaultMQTTReconnectInterval = original })

	// A very large failure count means Subscribe never succeeds on its own —
	// only closing shutdown can end the loop.
	fake := &fakePahoClient{subscribeFailures: 1 << 30}
	shutdown := make(chan struct{})
	c := &Client{
		pahoClient:        fake,
		logger:            logger.NewLogger("ERROR", io.Discard),
		subscribedTopic:   "#",
		subscribedHandler: func(_ string, _ []byte) {},
		shutdown:          shutdown,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.reconnectLoop()
	}()

	// Let a handful of failed attempts happen, then signal shutdown.
	time.Sleep(20 * time.Millisecond)
	close(shutdown)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reconnectLoop did not return after shutdown was closed")
	}

	if fake.subscribeCalls < 1 {
		t.Fatalf("Subscribe was never called before shutdown, want at least one attempt")
	}
}
