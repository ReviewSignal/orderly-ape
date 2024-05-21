package loadtesting

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/client"
	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/runtime"
)

var log = ctrl.Log.WithName("loadtesting-worker")

// Filter is a function that decides if an object should be sent as a reconcile event in k8s controllers.
type Filter func(object runtime.Object) bool

// Worker looks for changes from oxygen and pass them to the controller.
type Worker struct {
	C chan event.GenericEvent

	filters  []Filter
	client   client.Client
	resource string
}

// NewWorker instantiate a worker.
func NewWorker(client client.Client, obj runtime.Object, filters ...Filter) (*Worker, error) {
	kind := runtime.RealTypeOf(obj)

	w := &Worker{
		C:        make(chan event.GenericEvent),
		client:   client,
		resource: kind.String(),
		filters:  filters,
	}

	return w, nil
}

// Start watches for instances and send any changes to the controller. It's designed to be run by a manager.
func (w *Worker) Start(ctx context.Context) error {
	toWatch, err := runtime.Schema.NewObj(w.resource)
	if err != nil {
		log.Error(err, fmt.Sprintf("can't watch for %s", w.resource))
		return err
	}

	watcher, err := w.client.Watch(toWatch)
	if err != nil {
		log.Error(err, fmt.Sprintf("can't watch for %s", w.resource))
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case obj := <-watcher:
			if w.C != nil && w.isValid(obj) {
				w.C <- event.GenericEvent{
					Object: obj.ToK8SResource(),
				}
			}
		}
	}
}

func (w *Worker) isValid(obj runtime.Object) bool {
	for _, filter := range w.filters {
		if !filter(obj) {
			return false
		}
	}

	return true
}
