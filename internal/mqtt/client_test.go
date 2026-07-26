package mqtt

import (
	"io"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/spydemon/mqtt2bdd/internal/logger"
)

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

func (t *fakeToken) Wait() bool                       { return true }
func (t *fakeToken) WaitTimeout(_ time.Duration) bool { return true }
func (t *fakeToken) Done() <-chan struct{}            { return t.done }
func (t *fakeToken) Error() error                     { return t.err }

// fakePahoClient implements mqtt.Client just enough to drive reconnectLoop's
// Subscribe-retry behaviour: Connect always succeeds immediately, and Subscribe
// fails for subscribeFailures calls before succeeding, recording how many times
// it was invoked.
type fakePahoClient struct {
	subscribeFailures int
	subscribeCalls    int
}

func (f *fakePahoClient) IsConnected() bool      { return true }
func (f *fakePahoClient) IsConnectionOpen() bool { return true }
func (f *fakePahoClient) Connect() mqtt.Token    { return newFakeToken(nil) }
func (f *fakePahoClient) Disconnect(_ uint)      {}
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
