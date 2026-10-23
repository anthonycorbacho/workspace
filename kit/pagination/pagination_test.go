package pagination

import (
	"encoding/base64"
	"testing"

	pb "github.com/anthonycorbacho/workspace/kit/pagination/v1"
	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodeToken(t *testing.T) {
	var cases = []struct {
		name string
		in   *pb.PageIdentifier
		out  string
	}{
		{
			name: "should be able to encode resource and filter",
			in:   &pb.PageIdentifier{Name: "device/myuuid", Filter: "org_id = org/uuid"},
			out:  "Q2cxa1pYWnBZMlV2YlhsMWRXbGtFaEZ2Y21kZmFXUWdQU0J2Y21jdmRYVnBaQQ",
		},
		{
			name: "should be able to encode resource only",
			in:   &pb.PageIdentifier{Name: "device/myuuid"},
			out:  "Q2cxa1pYWnBZMlV2YlhsMWRXbGs",
		},
		{
			name: "should be able to encode filter only",
			in:   &pb.PageIdentifier{Filter: "org_id = org/uuid"},
		},
		{
			name: "should be able to encode empty name and filter",
			in:   &pb.PageIdentifier{},
			out:  "",
		},
		{
			name: "should be able to encode unicode and url unsafe characters",
			in:   &pb.PageIdentifier{Name: "device/é+?&=", Filter: "name = \"a b\" / ~"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotToken, err := EncodeToken(tc.in.Name, tc.in.Filter)
			if err != nil {
				assert.Fail(t, "", "EncodeToken: %v", err)
			}
			if tc.out != "" && tc.out != gotToken {
				assert.Fail(t, "", "EncodeToken want: %s, got %s", tc.out, gotToken)
			}
			gotName, gotFilter, err := DecodeToken(gotToken)
			if err != nil {
				assert.Fail(t, "", "DecodeToken: %v", err)
			}
			if (tc.in.Name != gotName) || (tc.in.Filter != gotFilter) {
				assert.Fail(t, "", "EncodeToken want: (%s, %s), got (%s, %s)", tc.in.Name, gotName, tc.in.Filter, gotFilter)
			}
		})
	}
}

func TestDecodeTokenInvalid(t *testing.T) {
	var cases = []struct {
		name  string
		token string
	}{
		{name: "not base64", token: "!!!"},
		{name: "standard base64 padding", token: "Q2c="},
		{name: "inner token not base64", token: base64.RawURLEncoding.EncodeToString([]byte("!!!"))},
		{name: "inner token not a protobuf message", token: base64.RawURLEncoding.EncodeToString(
			[]byte(base64.RawURLEncoding.EncodeToString([]byte{0xff, 0xff, 0xff})))},
		{name: "single encoded token", token: "Cg1kZXZpY2UvbXl1dWlk"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, filter, err := DecodeToken(tc.token)
			assert.Error(t, err)
			assert.Empty(t, name)
			assert.Empty(t, filter)
		})
	}
}

type batchSequence struct {
	want    int // what number we expect from this call to Next()
	fetched int // simulated number of returned results to feed into Update().
}

func TestBatcher(t *testing.T) {
	var cases = []struct {
		name     string
		pageSize int
		seq      []batchSequence
	}{
		{
			name:     "pageSize of 100 with 4 seq",
			pageSize: 100,
			seq: []batchSequence{
				{want: 100, fetched: 50},
				{want: 200, fetched: 10},
				{want: 1000, fetched: 10},
				{want: 1000}, // Caps at max
			},
		},
		{
			name:     "pageSize of 100 with 3 seq",
			pageSize: 100,
			seq: []batchSequence{
				{want: 100, fetched: 80},
				{want: 125, fetched: 20},
				{want: 625},
			},
		},
		{
			name:     "pageSize of 1",
			pageSize: 1,
			seq: []batchSequence{
				{want: 1, fetched: 5},
				{want: 1, fetched: 1},
				{want: 1},
			},
		},
		{
			name:     "every fetched item matched",
			pageSize: 100,
			seq: []batchSequence{
				{want: 100, fetched: 100},
				{want: 100, fetched: 100},
				{want: 100},
			},
		},
		{
			name:     "pageSize above max is capped",
			pageSize: 5000,
			seq: []batchSequence{
				{want: 1000, fetched: 1000},
				{want: 1000},
			},
		},
		{
			name:     "pageSize of 0",
			pageSize: 0,
			seq: []batchSequence{
				{want: 0, fetched: 0},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBatcher(tc.pageSize, 1000)
			for i, bs := range tc.seq {
				got := b.Next()
				if got != bs.want {
					assert.Fail(t, "", "step (%d) - want: %d, got %d", i, bs.want, got)
				}
				b.Update(bs.fetched, got)
			}
		})
	}
}

func TestBatcherUpdate(t *testing.T) {
	var cases = []struct {
		name    string
		matched int
		total   int
		want    int
	}{
		{name: "half matched doubles the batch", matched: 50, total: 100, want: 200},
		{name: "all matched", matched: 100, total: 100, want: 100},
		{name: "low ratio is capped at max", matched: 1, total: 1000, want: 1000},
		{name: "more matched than fetched shrinks the batch", matched: 200, total: 100, want: 50},
		// nothing matched in the last batch, fetch as much as allowed.
		{name: "nothing matched", matched: 0, total: 100, want: 1000},
		// nothing fetched in the last batch (empty page), the batcher must
		// still suggest a usable (positive) batch size.
		{name: "nothing fetched", matched: 0, total: 0, want: 1000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBatcher(100, 1000)
			b.Update(tc.matched, tc.total)
			got := b.Next()
			assert.Equal(t, tc.want, got)
			assert.Positive(t, got)
			assert.LessOrEqual(t, got, 1000)
		})
	}
}
