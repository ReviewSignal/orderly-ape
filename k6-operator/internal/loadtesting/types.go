package loadtesting

import (
	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/client"
)

type Client = client.Client
type StatusError = client.StatusError

var IsNotFound = client.IsNotFound
var IgnoreNotFound = client.IgnoreNotFound
