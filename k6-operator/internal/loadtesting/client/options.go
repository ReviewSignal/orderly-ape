package client

import (
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type options struct {
	URL      string
	Username string
	Password string

	Region string

	CacheRefreshInterval time.Duration
	Logger               logr.Logger
	StopCh               chan struct{}

	GenericEvents chan event.GenericEvent
}

func defaultOptions() *options {
	return &options{
		URL:                  "http://localhost:8000/api/",
		Username:             "admin",
		Password:             "admin",
		CacheRefreshInterval: 5 * time.Second,
	}
}

// Option is a function that configures the client.
type Option func(*options)

// WithGenericEvents sets the generic events channel where changes are sent.
func WithGenericEvents(events chan event.GenericEvent) Option {
	return func(args *options) {
		args.GenericEvents = events
	}
}

// WithCacheRefreshInterval sets the cache refresh interval.
func WithCacheRefreshInterval(seconds int) Option {
	return func(args *options) {
		args.CacheRefreshInterval = time.Duration(seconds) * time.Second
	}
}

// WithBaseURL to set the base URL
func WithBaseURL(baseURL string) Option {
	return func(args *options) {
		args.URL = baseURL
	}
}

// WithUsername sets the basic auth username.
func WithUsername(username string) Option {
	return func(args *options) {
		args.Username = username
	}
}

// WithPassword sets the basic auth password.
func WithPassword(password string) Option {
	return func(args *options) {
		args.Password = password
	}
}

// WithRegion sets the region.
func WithRegion(region string) Option {
	return func(args *options) {
		args.Region = region
	}
}

// WithLogger sets the logger.
func WithLogger(logger logr.Logger) Option {
	return func(args *options) {
		args.Logger = logger
	}
}
