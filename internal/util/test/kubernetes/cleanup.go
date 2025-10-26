// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package kubernetes

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Cleanup deletes a Kubernetes object, cleaning up specified finalizers if they exist.
// It ignores not found errors on both delete and get operations.
func Cleanup(ctx context.Context, c client.Client, obj client.Object, cleanupFinalizers ...string) error {
	// Try to get the object
	key := client.ObjectKeyFromObject(obj)
	if err := c.Get(ctx, key, obj); client.IgnoreNotFound(err) != nil {
		return err // ignored NotFound errors
	}

	// Remove specified finalizers if they exist
	if len(cleanupFinalizers) > 0 {
		modified := false
		for _, finalizer := range cleanupFinalizers {
			if controllerutil.RemoveFinalizer(obj, finalizer) {
				modified = true
			}
		}
		if modified {
			if err := c.Update(ctx, obj); client.IgnoreNotFound(err) != nil {
				return err
			}
		}
	}

	// Delete the object
	err := c.Delete(ctx, obj, client.PropagationPolicy(metav1.DeletePropagationBackground))
	return client.IgnoreNotFound(err)
}
