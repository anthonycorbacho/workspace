package kafka

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestDeadLetterValidate(t *testing.T) {
	tests := []struct {
		name    string
		dl      deadLetter
		wantErr string
	}{
		{name: "disabled"},
		{name: "enabled", dl: deadLetter{topic: "{topic}.dlq", maxAttempts: 3}},
		{name: "missing topic", dl: deadLetter{maxAttempts: 3}, wantErr: "topic is required"},
		{name: "invalid max attempts", dl: deadLetter{topic: "dlq"}, wantErr: "at least 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.dl.validate()
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				_, err = NewClient([]string{unreachableBroker}, WithDeadLetter(tt.dl.topic, tt.dl.maxAttempts))
				assert.ErrorContains(t, err, tt.wantErr, "NewClient validates the dead letter")
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestDeadLetterTopicOf(t *testing.T) {
	tests := []struct {
		name  string
		topic string
		want  string
	}{
		{name: "placeholder", topic: "{topic}.dlq", want: "orders.dlq"},
		{name: "fixed topic", topic: "dead-letters", want: "dead-letters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, deadLetter{topic: tt.topic, maxAttempts: 1}.topicOf("orders"))
		})
	}
}

func TestDeadLetterRecord(t *testing.T) {
	rec := &kgo.Record{
		Topic: "orders", Partition: 2, Offset: 42, Key: []byte("k"), Value: []byte("v"),
		Headers: []kgo.RecordHeader{
			{Key: "attribute", Value: []byte("a")},
			// a record consumed from a dead letter topic is dead lettered with the last failure.
			{Key: deadLetterAttemptsHeader, Value: []byte("9")},
		},
	}
	dl := deadLetter{topic: "{topic}.dlq", maxAttempts: 3}.record(rec, 3, errors.New("boom"))

	assert.Equal(t, "orders.dlq", dl.Topic)
	assert.Equal(t, []byte("k"), dl.Key)
	assert.Equal(t, []byte("v"), dl.Value)
	headers := map[string]string{}
	for _, h := range dl.Headers {
		_, dup := headers[h.Key]
		require.False(t, dup, "header %s is duplicated", h.Key)
		headers[h.Key] = string(h.Value)
	}
	assert.Equal(t, map[string]string{
		"attribute":               "a",
		deadLetterTopicHeader:     "orders",
		deadLetterPartitionHeader: "2",
		deadLetterOffsetHeader:    "42",
		deadLetterAttemptsHeader:  "3",
		deadLetterErrorHeader:     "boom",
	}, headers)
	assert.Len(t, rec.Headers, 2, "the consumed record is unchanged")
}
