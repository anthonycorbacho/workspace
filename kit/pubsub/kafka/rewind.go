package kafka

import (
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

// topicPartition identifies a partition.
type topicPartition struct {
	topic     string
	partition int32
}

// partitionRewind tracks the nacks of a partition.
type partitionRewind struct {
	// generation is bumped by each nack, the records delivered before are consumed again.
	generation uint64
	// offset is the offset to consume the partition from.
	offset kgo.EpochOffset
	// pending reports whether the partition must be rewound to offset after the current fetch.
	pending bool
	// failures is the number of failed deliveries of the record at failedOffset, the record the
	// partition is rewound to, it is reset once the record is acked.
	failures     int
	failedOffset int64
}

// delivery is a delivery of a record.
type delivery struct {
	// generation is the generation of the record partition when the record was delivered.
	generation uint64
	// attempt is the delivery attempt of the record, from 1.
	attempt int
}

// rewinder rewinds the partitions of the nacked records.
//
// Kafka acknowledges a record by committing the offset following it, which acknowledges the
// previous records of the partition too. So a nacked record is consumed again with the records
// following it, and the acks of the records delivered before a nack are ignored: committing
// them could commit past the nacked record.
type rewinder struct {
	mu         sync.Mutex
	partitions map[topicPartition]*partitionRewind
}

func newRewinder() *rewinder {
	return &rewinder{partitions: map[topicPartition]*partitionRewind{}}
}

// deliver returns the delivery of the record, skip reports whether the record must not be handled
// as its partition is going to be rewound before it.
func (r *rewinder) deliver(rec *kgo.Record) (d delivery, skip bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.partitions[topicPartition{rec.Topic, rec.Partition}]
	if !ok {
		return delivery{attempt: 1}, false
	}
	d = delivery{generation: p.generation, attempt: 1}
	if p.failures > 0 && rec.Offset == p.failedOffset {
		d.attempt = p.failures + 1
	}
	return d, p.pending
}

// ack calls commit unless the partition of the record delivered at generation was rewound since.
func (r *rewinder) ack(rec *kgo.Record, generation uint64, commit func()) {
	// held while committing so a concurrent nack cannot be committed past.
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.partitions[topicPartition{rec.Topic, rec.Partition}]
	if ok && p.generation != generation {
		return
	}
	commit()
	if ok && rec.Offset >= p.failedOffset {
		p.failures = 0
	}
}

// nack rewinds the partition of the record delivered at generation to the record.
func (r *rewinder) nack(rec *kgo.Record, generation uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tp := topicPartition{rec.Topic, rec.Partition}
	p, ok := r.partitions[tp]
	if !ok {
		p = &partitionRewind{}
		r.partitions[tp] = p
	}
	// a record delivered before the last rewind is already consumed again, unless it precedes
	// the rewind; rewinding forward would skip records.
	if p.generation != generation && rec.Offset >= p.offset.Offset {
		return
	}
	if !p.pending || rec.Offset < p.offset.Offset {
		p.offset = kgo.EpochOffset{Epoch: rec.LeaderEpoch, Offset: rec.Offset}
	}
	p.pending = true
	p.generation++

	// the failures are counted for the record the partition is rewound to.
	if p.failures > 0 && p.failedOffset == p.offset.Offset {
		if rec.Offset == p.failedOffset {
			p.failures++
		}
		return
	}
	p.failedOffset = p.offset.Offset
	p.failures = 0
	if rec.Offset == p.failedOffset {
		p.failures = 1
	}
}

// failures returns the number of failed deliveries of the record the partition is rewound to.
func (r *rewinder) failures(topic string, partition int32) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p, ok := r.partitions[topicPartition{topic, partition}]; ok {
		return p.failures
	}
	return 0
}

// rewind returns the offsets the partitions must be consumed from, the rewinds are then
// considered done.
func (r *rewinder) rewind() map[string]map[int32]kgo.EpochOffset {
	r.mu.Lock()
	defer r.mu.Unlock()

	var offsets map[string]map[int32]kgo.EpochOffset
	for tp, p := range r.partitions {
		if !p.pending {
			continue
		}
		p.pending = false
		if offsets == nil {
			offsets = map[string]map[int32]kgo.EpochOffset{}
		}
		if offsets[tp.topic] == nil {
			offsets[tp.topic] = map[int32]kgo.EpochOffset{}
		}
		offsets[tp.topic][tp.partition] = p.offset
	}
	return offsets
}
