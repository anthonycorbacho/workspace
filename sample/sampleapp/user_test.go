package sampleapp_test

import (
	"context"
	"testing"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/sample/sampleapp"
	"github.com/anthonycorbacho/workspace/sample/sampleapp/inmem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAndFetch(t *testing.T) {
	service := sampleapp.New(inmem.New())
	ctx := context.Background()

	u := sampleapp.User{Name: "Toto"}
	require.NoError(t, service.Create(ctx, &u))
	assert.NotEmpty(t, u.ID)

	got, err := service.Fetch(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, &u, got)
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name     string
		run      func(ctx context.Context, service *sampleapp.UserService) error
		wantCode string
	}{
		{
			name: "create without name",
			run: func(ctx context.Context, service *sampleapp.UserService) error {
				return service.Create(ctx, &sampleapp.User{})
			},
			wantCode: errors.InvalidRequestCode,
		},
		{
			name: "create a user with an existing name",
			run: func(ctx context.Context, service *sampleapp.UserService) error {
				require.NoError(t, service.Create(ctx, &sampleapp.User{Name: "Toto"}))
				return service.Create(ctx, &sampleapp.User{Name: "Toto"})
			},
			wantCode: errors.SampleUserAlreadyExistsCode,
		},
		{
			name: "fetch an unknown user",
			run: func(ctx context.Context, service *sampleapp.UserService) error {
				_, err := service.Fetch(ctx, "unknown")
				return err
			},
			wantCode: errors.SampleUserNotFoundCode,
		},
		{
			name: "delete an unknown user",
			run: func(ctx context.Context, service *sampleapp.UserService) error {
				return service.Delete(ctx, "unknown")
			},
			wantCode: errors.SampleUserNotFoundCode,
		},
		{
			name: "fetch a deleted user",
			run: func(ctx context.Context, service *sampleapp.UserService) error {
				u := sampleapp.User{Name: "Toto"}
				require.NoError(t, service.Create(ctx, &u))
				require.NoError(t, service.Delete(ctx, u.ID))
				_, err := service.Fetch(ctx, u.ID)
				return err
			},
			wantCode: errors.SampleUserNotFoundCode,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(context.Background(), sampleapp.New(inmem.New()))
			assert.True(t, errors.HasCode(err, tt.wantCode), "want %s, got %v", tt.wantCode, err)
		})
	}
}

func TestFetchReturnsACopy(t *testing.T) {
	service := sampleapp.New(inmem.New())
	ctx := context.Background()
	u := sampleapp.User{Name: "Toto"}
	require.NoError(t, service.Create(ctx, &u))

	got, err := service.Fetch(ctx, u.ID)
	require.NoError(t, err)
	got.Name = "changed"

	again, err := service.Fetch(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "Toto", again.Name, "the stored user is not modified")
}
