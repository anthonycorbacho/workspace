package mqtt

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/eclipse/paho.golang/packets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mqttTestTimeout = 3 * time.Second

// mqttPeer speaks enough MQTT v5 to control connection loss and subscription acknowledgements.
// The client uses its real TCP connection, reconnect loop and message router in these tests.
type mqttPeer struct {
	listener *net.TCPListener
}

func newMQTTPeer(t *testing.T) *mqttPeer {
	t.Helper()
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	return &mqttPeer{listener: listener}
}

func (p *mqttPeer) accept(t *testing.T) net.Conn {
	t.Helper()
	require.NoError(t, p.listener.SetDeadline(time.Now().Add(mqttTestTimeout)))
	conn, err := p.listener.Accept()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	readMQTTPacket(t, conn, packets.CONNECT)
	// Every connection has a fresh broker session, as with the default session expiry of zero.
	writeMQTTPacket(t, conn, &packets.Connack{Properties: &packets.Properties{}})
	return conn
}

func readMQTTPacket(t *testing.T, conn net.Conn, packetType byte) *packets.ControlPacket {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(mqttTestTimeout)))
	for {
		packet, err := packets.ReadPacket(conn)
		require.NoError(t, err, "waiting for MQTT packet type %d", packetType)
		if packet.Type == packets.PINGREQ {
			writeMQTTPacket(t, conn, &packets.Pingresp{})
			continue
		}
		require.Equal(t, packetType, packet.Type)
		return packet
	}
}

func writeMQTTPacket(t *testing.T, conn net.Conn, packet packets.Packet) {
	t.Helper()
	require.NoError(t, conn.SetWriteDeadline(time.Now().Add(mqttTestTimeout)))
	_, err := packet.WriteTo(conn)
	require.NoError(t, err)
}

func awaitMQTTResult[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(mqttTestTimeout):
		t.Fatal("timed out waiting for MQTT operation")
		var zero T
		return zero
	}
}

func newMQTTTestClient(t *testing.T) (*Client, *mqttPeer, net.Conn) {
	t.Helper()
	peer := newMQTTPeer(t)
	type result struct {
		client *Client
		err    error
	}
	connected := make(chan result, 1)
	ctx, cancel := context.WithTimeout(context.Background(), mqttTestTimeout)
	defer cancel()
	go func() {
		client, err := NewClient(ctx, "mqtt://"+peer.listener.Addr().String(), "reconnect-test", WithRetry(10*time.Millisecond))
		connected <- result{client: client, err: err}
	}()
	conn := peer.accept(t)
	r := awaitMQTTResult(t, connected)
	require.NoError(t, r.err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), mqttTestTimeout)
		defer cancel()
		assert.NoError(t, r.client.Disconnect(ctx))
	})
	return r.client, peer, conn
}

func ackMQTTSubscription(t *testing.T, conn net.Conn, sub *packets.Subscribe, reasons ...byte) {
	t.Helper()
	writeMQTTPacket(t, conn, &packets.Suback{
		PacketID: sub.PacketID, Properties: &packets.Properties{}, Reasons: reasons,
	})
}

func TestSubscriberRestoresSubscriptions(t *testing.T) {
	client, peer, conn := newMQTTTestClient(t)
	subscriber, err := NewSubscriber(client)
	require.NoError(t, err)

	type contextKey struct{}
	type received struct {
		topic string
		body  string
		value any
		err   error
	}
	messages := make(chan received, 16)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "kept"))
	defer cancel()
	handler := func(ctx context.Context, msg pubsub.Message, _ map[string]string) error {
		messages <- received{topic: pubsub.Topic(ctx), body: msg.String(), value: ctx.Value(contextKey{}), err: ctx.Err()}
		return nil
	}
	subscriptions := []packets.SubOptions{
		{Topic: "kit/reconnect/exact", QoS: 2, RetainHandling: 2},
		{Topic: "kit/reconnect/+/status", QoS: 2, RetainHandling: 2},
	}
	for _, options := range subscriptions {
		subscribed := make(chan error, 1)
		go func() { subscribed <- subscriber.Subscribe(ctx, options.Topic, handler) }()
		sub := readMQTTPacket(t, conn, packets.SUBSCRIBE).Content.(*packets.Subscribe)
		assert.Equal(t, []packets.SubOptions{options}, sub.Subscriptions)
		ackMQTTSubscription(t, conn, sub, packets.SubackGrantedQoS2)
		require.NoError(t, awaitMQTTResult(t, subscribed))
	}
	// The caller's subscribe context bounds registration, not future reconnections or handlers.
	cancel()

	for connection := range 3 {
		if connection > 0 {
			require.NoError(t, conn.Close())
			conn = peer.accept(t)
			sub := readMQTTPacket(t, conn, packets.SUBSCRIBE).Content.(*packets.Subscribe)
			assert.ElementsMatch(t, subscriptions, sub.Subscriptions)
			ackMQTTSubscription(t, conn, sub, packets.SubackGrantedQoS2, packets.SubackGrantedQoS2)
		}
		for i, topic := range []string{"kit/reconnect/exact", "kit/reconnect/device/status"} {
			packetID := uint16(i + 1)
			writeMQTTPacket(t, conn, &packets.Publish{
				Topic: topic, QoS: 1, PacketID: packetID, Payload: []byte("hello"), Properties: &packets.Properties{},
			})
			// PUBACK is sent after the router has called all matching handlers.
			ack := readMQTTPacket(t, conn, packets.PUBACK)
			assert.Equal(t, packetID, ack.PacketID())
			assert.Equal(t, received{topic: topic, body: "hello", value: "kept"}, awaitMQTTResult(t, messages))
			select {
			case extra := <-messages:
				t.Fatalf("reconnection registered a duplicate handler: %+v", extra)
			default:
			}
		}
	}
}

func TestSubscriberRetriesRestoration(t *testing.T) {
	client, peer, conn := newMQTTTestClient(t)
	subscriber, err := NewSubscriber(client)
	require.NoError(t, err)
	handler := func(context.Context, pubsub.Message, map[string]string) error { return nil }
	subscriptions := []packets.SubOptions{
		{Topic: "kit/retry/one", QoS: 2, RetainHandling: 2},
		{Topic: "kit/retry/two", QoS: 2, RetainHandling: 2},
	}
	for _, topic := range []string{"kit/retry/one", "kit/retry/two", "kit/retry/denied"} {
		subscribed := make(chan error, 1)
		go func() { subscribed <- subscriber.Subscribe(context.Background(), topic, handler) }()
		sub := readMQTTPacket(t, conn, packets.SUBSCRIBE).Content.(*packets.Subscribe)
		if topic == "kit/retry/denied" {
			ackMQTTSubscription(t, conn, sub, packets.SubackNotauthorized)
			require.Error(t, awaitMQTTResult(t, subscribed))
		} else {
			ackMQTTSubscription(t, conn, sub, packets.SubackGrantedQoS2)
			require.NoError(t, awaitMQTTResult(t, subscribed))
		}
	}

	require.NoError(t, conn.Close())
	conn = peer.accept(t)
	sub := readMQTTPacket(t, conn, packets.SUBSCRIBE).Content.(*packets.Subscribe)
	assert.ElementsMatch(t, subscriptions, sub.Subscriptions, "a rejected registration must not be restored")
	// A partially rejected SUBACK must trigger another attempt on the same connection.
	ackMQTTSubscription(t, conn, sub, packets.SubackGrantedQoS2, packets.SubackUnspecifiederror)
	sub = readMQTTPacket(t, conn, packets.SUBSCRIBE).Content.(*packets.Subscribe)
	assert.ElementsMatch(t, subscriptions, sub.Subscriptions)

	// Losing the connection while waiting for SUBACK must not lose the remembered subscriptions.
	require.NoError(t, conn.Close())
	conn = peer.accept(t)
	sub = readMQTTPacket(t, conn, packets.SUBSCRIBE).Content.(*packets.Subscribe)
	assert.ElementsMatch(t, subscriptions, sub.Subscriptions)
	ackMQTTSubscription(t, conn, sub, packets.SubackGrantedQoS2, packets.SubackGrantedQoS2)
}

func TestSubscriberRetriesRejectedHandler(t *testing.T) {
	client, _, conn := newMQTTTestClient(t)
	subscriber, err := NewSubscriber(client)
	require.NoError(t, err)
	const topic = "kit/registration/retry"
	deliveries := make(chan string, 8)
	subscribe := func(name string) (*packets.Subscribe, <-chan error) {
		t.Helper()
		result := make(chan error, 1)
		go func() {
			result <- subscriber.Subscribe(t.Context(), topic, func(context.Context, pubsub.Message, map[string]string) error {
				deliveries <- name
				return nil
			})
		}()
		return readMQTTPacket(t, conn, packets.SUBSCRIBE).Content.(*packets.Subscribe), result
	}
	publish := func(packetID uint16, want ...string) {
		t.Helper()
		writeMQTTPacket(t, conn, &packets.Publish{Topic: topic, QoS: 1, PacketID: packetID, Properties: &packets.Properties{}})
		// The broker sees PUBACK only after all the matching handlers have returned.
		readMQTTPacket(t, conn, packets.PUBACK)
		var got []string
		for range len(deliveries) {
			got = append(got, <-deliveries)
		}
		assert.ElementsMatch(t, want, got)
	}
	sub, done := subscribe("existing")
	publish(1, "existing") // MQTT allows PUBLISH before SUBACK.
	ackMQTTSubscription(t, conn, sub, packets.SubackGrantedQoS2)
	require.NoError(t, awaitMQTTResult(t, done))
	sub, done = subscribe("rejected")
	ackMQTTSubscription(t, conn, sub, packets.SubackNotauthorized)
	require.Error(t, awaitMQTTResult(t, done))
	sub, done = subscribe("retried")
	ackMQTTSubscription(t, conn, sub, packets.SubackGrantedQoS2)
	require.NoError(t, awaitMQTTResult(t, done))
	publish(2, "existing", "retried")
}

func TestSubscriberCloseDuringRestoration(t *testing.T) {
	client, peer, conn := newMQTTTestClient(t)
	subscriber, err := NewSubscriber(client)
	require.NoError(t, err)
	handler := func(context.Context, pubsub.Message, map[string]string) error { return nil }
	subscribed := make(chan error, 1)
	go func() { subscribed <- subscriber.Subscribe(context.Background(), "kit/close", handler) }()
	sub := readMQTTPacket(t, conn, packets.SUBSCRIBE).Content.(*packets.Subscribe)
	ackMQTTSubscription(t, conn, sub, packets.SubackGrantedQoS2)
	require.NoError(t, awaitMQTTResult(t, subscribed))

	require.NoError(t, conn.Close())
	conn = peer.accept(t)
	readMQTTPacket(t, conn, packets.SUBSCRIBE)
	// Leave the restoration waiting for SUBACK. Close must cancel it and join the worker promptly.
	closed := make(chan error, 1)
	go func() { closed <- subscriber.Close() }()
	readMQTTPacket(t, conn, packets.DISCONNECT)
	require.NoError(t, awaitMQTTResult(t, closed))
	require.NoError(t, subscriber.Close())
	assert.ErrorIs(t, subscriber.Subscribe(context.Background(), "kit/close", handler), pubsub.ErrSubscriberClosed)
}
