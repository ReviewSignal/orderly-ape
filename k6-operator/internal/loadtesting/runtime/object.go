package runtime

import "sigs.k8s.io/controller-runtime/pkg/client"

// Object represents an interface for a generic REST resource.
// It provides translation between the REST resource and the kubernetes object.
type Object interface {
	ToK8SResource() client.Object
	GetName() string
}

type ObjectList interface {
	GetItem() Object
	SetItems([]Object)
}
