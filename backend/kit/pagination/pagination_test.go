package pagination

import (
	"testing"

	pb "github.com/anthonycorbacho/workspace/kit/pagination/v1"
	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodeToken(t *testing.T) {
	var cases = []struct {
		name string
		in   pb.PageIdentifier
		out  string
	}{
		{
			name: "should be able to encode resource and filter",
			in:   pb.PageIdentifier{Name: "device/myuuid", Filter: "org_id = org/uuid"},
			out:  "Q2cxa1pYWnBZMlV2YlhsMWRXbGtFaEZ2Y21kZmFXUWdQU0J2Y21jdmRYVnBaQQ",
		},
		{
			name: "should be able to encode resource only",
			in:   pb.PageIdentifier{Name: "device/myuuid"},
			out:  "Q2cxa1pYWnBZMlV2YlhsMWRXbGs",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotToken, err := EncodeToken(tc.in.Name, tc.in.Filter)
			if err != nil {
				assert.Fail(t, "", "EncodeToken: %v", err)
			}
			if tc.out != gotToken {
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
