package loadtesting

import (
	"context"
	"time"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/api"
	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/client"
)

// Pinger checks-in with Orderly Ape webapp
type Pinger struct {
	client client.Client
}

// NewWorker instantiate a pinger
func NewPinger(client client.Client) (*Pinger, error) {
	p := &Pinger{
		client: client,
	}

	return p, nil
}

// Start watches for instances and send any changes to the controller. It's designed to be run by a manager.
func (p *Pinger) Start(ctx context.Context) error {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			p.Ping(ctx)
		}
	}
}

func (p *Pinger) Ping(ctx context.Context) {
	log.Info("ping home")
	p.client.Update(ctx, &api.Ping{})
}
