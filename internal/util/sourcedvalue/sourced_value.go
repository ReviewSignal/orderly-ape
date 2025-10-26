// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package sourcedvalue

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

func Get(ctx context.Context, c client.Client, ns string, value apev1.SourcedValue) (string, error) {
	switch {
	case value.ValueFrom != nil && value.ValueFrom.SecretKeyRef != nil:
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      value.ValueFrom.SecretKeyRef.Name,
				Namespace: ns,
			},
		}
		if err := c.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
			return "", fmt.Errorf("failed sourcing key '%s' from secret '%s': %w", value.ValueFrom.SecretKeyRef.Key, value.ValueFrom.SecretKeyRef.Name, err)
		}
		return string(secret.Data[value.ValueFrom.SecretKeyRef.Key]), nil
	case value.ValueFrom != nil && value.ValueFrom.ConfigMapKeyRef != nil:
		cm := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      value.ValueFrom.ConfigMapKeyRef.Name,
				Namespace: ns,
			},
		}
		if err := c.Get(ctx, client.ObjectKeyFromObject(cm), cm); err != nil {
			return "", fmt.Errorf("failed sourcing key '%s' from config map '%s': %w", value.ValueFrom.ConfigMapKeyRef.Key, value.ValueFrom.ConfigMapKeyRef.Name, err)
		}
		return string(cm.Data[value.ValueFrom.SecretKeyRef.Key]), nil
	default:
		return value.Value, nil
	}
}
