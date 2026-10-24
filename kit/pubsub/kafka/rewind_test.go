package kafka

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestRewinder(t *testing.T) {
	type op int
	const (
		deliver op = iota
		ack
		nack
		rewind
	)
	type step struct {
		op        op
		partition int32
		offset    int64
	}
	at := func(o op, offset int64) step { return step{op: o, offset: offset} }
	// the partitions are rewound once the records of a fetch are handled.
	endOfFetch := step{op: rewind}
	type rewinds = map[string]map[int32]kgo.EpochOffset
	rewindTo := func(partition int32, offset int64) rewinds {
		return rewinds{"t": {partition: {Epoch: 0, Offset: offset}}}
	}

	tests := []struct {
		name          string
		steps         []step
		wantCommitted []string
		wantSkipped   []string
		wantRewinds   []rewinds
	}{
		{
			name:          "acks commit",
			steps:         []step{at(deliver, 0), at(ack, 0), at(deliver, 1), at(ack, 1), endOfFetch},
			wantCommitted: []string{"0/0", "0/1"},
			wantRewinds:   []rewinds{nil},
		},
		{
			name: "nack rewinds to the record and skips the next ones",
			steps: []step{
				at(deliver, 0), at(ack, 0), at(deliver, 1), at(nack, 1), at(deliver, 2), endOfFetch,
				at(deliver, 1), at(ack, 1), at(deliver, 2), at(ack, 2), endOfFetch,
			},
			wantCommitted: []string{"0/0", "0/1", "0/2"},
			wantSkipped:   []string{"0/2"},
			wantRewinds:   []rewinds{rewindTo(0, 1), nil},
		},
		{
			name: "ack of a record delivered before a nack is ignored",
			steps: []step{
				at(deliver, 0), at(deliver, 1), at(nack, 0), at(ack, 1), endOfFetch,
				at(deliver, 0), at(ack, 0), at(deliver, 1), at(ack, 1),
			},
			wantCommitted: []string{"0/0", "0/1"},
			wantRewinds:   []rewinds{rewindTo(0, 0)},
		},
		{
			name: "late nack of a preceding record rewinds further",
			steps: []step{
				at(deliver, 0), at(deliver, 1), at(nack, 1), endOfFetch, at(nack, 0), endOfFetch,
			},
			wantRewinds: []rewinds{rewindTo(0, 1), rewindTo(0, 0)},
		},
		{
			name: "nacks before a rewind rewind to the first record",
			steps: []step{
				at(deliver, 0), at(deliver, 1), at(nack, 1), at(nack, 0), endOfFetch,
			},
			wantRewinds: []rewinds{rewindTo(0, 0)},
		},
		{
			name: "late nack of a following record is ignored",
			steps: []step{
				at(deliver, 0), at(deliver, 1), at(nack, 0), endOfFetch, at(nack, 1), endOfFetch,
			},
			wantRewinds: []rewinds{rewindTo(0, 0), nil},
		},
		{
			name: "partitions are independent",
			steps: []step{
				{op: deliver, partition: 0, offset: 0}, {op: deliver, partition: 1, offset: 0},
				{op: nack, partition: 0, offset: 0},
				{op: deliver, partition: 0, offset: 1}, {op: deliver, partition: 1, offset: 1},
				{op: ack, partition: 1, offset: 0}, {op: ack, partition: 1, offset: 1},
				endOfFetch,
			},
			wantCommitted: []string{"1/0", "1/1"},
			wantSkipped:   []string{"0/1"},
			wantRewinds:   []rewinds{rewindTo(0, 0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRewinder()
			// the generation of the last delivery of each record.
			generations := map[string]uint64{}
			var committed, skipped []string
			var gotRewinds []rewinds

			for _, s := range tt.steps {
				key := fmt.Sprintf("%d/%d", s.partition, s.offset)
				rec := &kgo.Record{Topic: "t", Partition: s.partition, Offset: s.offset}
				switch s.op {
				case deliver:
					generation, skip := r.deliver(rec)
					if skip {
						skipped = append(skipped, key)
					}
					generations[key] = generation
				case ack:
					generation, ok := generations[key]
					require.True(t, ok, "%s is not delivered", key)
					r.ack(rec, generation, func() { committed = append(committed, key) })
				case nack:
					generation, ok := generations[key]
					require.True(t, ok, "%s is not delivered", key)
					r.nack(rec, generation)
				case rewind:
					gotRewinds = append(gotRewinds, r.rewind())
				}
			}

			assert.Equal(t, tt.wantCommitted, committed)
			assert.Equal(t, tt.wantSkipped, skipped)
			assert.Equal(t, tt.wantRewinds, gotRewinds)
		})
	}
}
