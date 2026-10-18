package testutil

import (
	"context"
	"testing"

	"golang.org/x/sync/errgroup"
)

type TestEnvironment struct {
	Eg     *errgroup.Group
	Ctx    context.Context
	Cancel context.CancelFunc
	T      *testing.T
}

func NewTestEnvironment(t *testing.T) *TestEnvironment {
	eg, ctx := errgroup.WithContext(context.Background())
	ctx, cancel := context.WithCancel(ctx)

	return &TestEnvironment{
		Eg:     eg,
		Ctx:    ctx,
		Cancel: cancel,
		T:      t,
	}
}
