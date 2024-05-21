package client

import (
	"context"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/runtime"
)

type Client interface {
	Get(ctx context.Context, id string, obj runtime.Object) error
	List(ctx context.Context, obj runtime.ObjectList) error
	Watch(obj runtime.Object) (<-chan runtime.Object, error)
}
