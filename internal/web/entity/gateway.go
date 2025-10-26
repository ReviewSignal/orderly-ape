// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package entity

import (
	"net/url"

	"github.com/a-h/templ"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Gateway struct {
	*gatewayv1.Gateway
}

type GatewayList []Gateway

func WrapGateway(gw *gatewayv1.Gateway) Gateway {
	return Gateway{gw}
}

func WrapGatewayList(items []gatewayv1.Gateway) GatewayList {
	wrapped := make(GatewayList, len(items))
	for i, item := range items {
		wrapped[i] = WrapGateway(&item)
	}
	return wrapped
}

func (g Gateway) NamespacedName() string {
	return client.ObjectKeyFromObject(g.Gateway).String()
}

func (g Gateway) ObjectLink() templ.SafeURL {
	path, _ := url.JoinPath("gateways", g.Name)
	return templ.URL(path)
}

func (g Gateway) VerboseName() string {
	return "Gateway"
}
