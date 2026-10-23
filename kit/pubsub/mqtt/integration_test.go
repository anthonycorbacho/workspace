package mqtt

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	kitConfig "github.com/anthonycorbacho/workspace/kit/config"
	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/eclipse/paho.golang/paho"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type mqttTestSuite struct {
	suite.Suite
	testURL string
}

func TestMqttTestSuite(t *testing.T) {
	suite.Run(t, new(mqttTestSuite))
}

func (m *mqttTestSuite) SetupSuite() {
	if os.Getenv("TESTINGMQTT_URL") == "" {
		m.T().Skip("No testing mqtt setup via env variable TESTINGMQTT_URL")
	}
	m.testURL = kitConfig.LookupEnv("TESTINGMQTT_URL", "mqtt://127.0.0.1:1883")
}

func (m *mqttTestSuite) TestClient() {
	clientId := fmt.Sprintf("test-client-%s", id.New())
	c, err := NewClient(m.testURL, clientId)
	m.NoError(err)
	defer c.Disconnect(context.Background())

	// Try to publish to make sure we dont get any error.
	_, err = c.Publish(context.Background(), &paho.Publish{
		Topic:   clientId,
		Payload: []byte("whatever."),
	})

	m.NoError(err)
}

func (m *mqttTestSuite) TestPublishAndSubscribe() {
	clientId := fmt.Sprintf("test-client-%s", id.New())
	c, err := NewClient(m.testURL, clientId)
	m.NoError(err)
	defer c.Disconnect(context.Background())

	publisher, err := NewPublisher(c)
	m.NoError(err)
	defer publisher.Close()

	// Custom sub

	subscriber, err := NewSubscriber(c)
	m.NoError(err)
	defer subscriber.Close()

	choke := make(chan string, 1)
	fn := func(ctx context.Context, msg pubsub.Message, meta map[string]string) error {
		choke <- string(msg)
		return nil
	}

	err = subscriber.Subscribe(context.Background(), clientId, fn)
	m.NoError(err)

	err = publisher.Publish(
		context.Background(),
		clientId,
		[]byte("a message"),
		pubsub.WithQoS(0),
		pubsub.WithAttributes(map[string]string{"attb1": "value"}),
	)
	m.NoError(err)

	select {
	case result := <-choke:
		assert.Equal(m.T(), "a message", result)
	case <-time.After(time.Second * 10):
		assert.Fail(m.T(), "timeout")

	}
}
