package kafka

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	kitConfig "github.com/anthonycorbacho/workspace/kit/config"
	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type kafkaTestSuite struct {
	suite.Suite
	testURL string
}

func TestKafkaTestSuite(t *testing.T) {
	suite.Run(t, new(kafkaTestSuite))
}

func (suite *kafkaTestSuite) SetupSuite() {
	if os.Getenv("TESTINGKAFKA_URL") == "" {
		suite.T().Skip("No testing kafka setup via env variable TESTINGKAFKA_URL")
	}
	suite.testURL = kitConfig.LookupEnv("TESTINGKAFKA_URL", "127.0.0.1:19092")

}

func (suite *kafkaTestSuite) TestClient() {
	clientId := fmt.Sprintf("test-client-%s", id.New())
	c, err := NewClient([]string{suite.testURL}, WithClientID(clientId))
	suite.NoError(err)
	defer c.Close()
	suite.NoError(c.Ping())
}

func (suite *kafkaTestSuite) TestPublishAndSubscribe() {
	clientId := fmt.Sprintf("test-client-%s", id.New())
	c, err := NewClient([]string{suite.testURL}, WithClientID(clientId), WithAutoTopicCreation())
	suite.NoError(err)
	defer c.Close()

	publisher, err := NewPublisher(c)
	suite.NoError(err)
	defer publisher.Close()

	// Custom sub

	subscriber, err := NewSubscriber(c)
	suite.NoError(err)
	defer subscriber.Close()

	choke := make(chan string, 1)
	fn := func(ctx context.Context, msg pubsub.Message, meta map[string]string) error {
		choke <- string(msg)
		return nil
	}

	err = subscriber.Subscribe(context.Background(), "my-topic-1", fn)
	suite.NoError(err)
	defer subscriber.Close()

	err = publisher.Publish(
		context.Background(),
		"my-topic-1",
		[]byte("a message"),
		pubsub.WithKey("company_id_1"),
		pubsub.WithAttributes(map[string]string{"attb1": "value"}),
	)
	suite.NoError(err)

	select {
	case result := <-choke:
		assert.Equal(suite.T(), "a message", result)
	case <-time.After(time.Second * 10):
		assert.Fail(suite.T(), "timeout")

	}
}
