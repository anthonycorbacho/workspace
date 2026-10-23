package errors

import (
	"testing"

	errdetails "github.com/anthonycorbacho/workspace/api/errdetails/v1"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStatus(t *testing.T) {
	var cases = []struct {
		name    string
		code    codes.Code
		msg     string
		details *errdetails.ErrorInfo
	}{
		{
			name: "should be able to set Status error",
			code: codes.InvalidArgument,
			msg:  "Bad argument 1",
		},
		{
			name: "should be able to set Status error with details",
			code: codes.InvalidArgument,
			msg:  "Bad argument",
			details: &errdetails.ErrorInfo{
				Reason: "MISSING_ARGUMENT",
				Metadata: map[string]string{
					"A": "B",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var details []*errdetails.ErrorInfo
			if tc.details != nil {
				details = append(details, tc.details)
			}
			err := Status(tc.code, tc.msg, details...)
			assert.NotNil(t, err)

			st := status.Convert(err)
			assert.NotNil(t, st)
			assert.Equal(t, tc.code, st.Code())
			assert.Equal(t, tc.msg, st.Message())

			if len(st.Details()) > 0 {
				_, ok := st.Details()[0].(*errdetails.ErrorInfo)
				assert.True(t, ok)
			}
		})
	}
}
