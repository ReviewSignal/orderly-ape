// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package url

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/a-h/templ"
	"github.com/gorilla/mux"
)

type urlKey string

var (
	requestURLKey urlKey = "request"
	routerKey     urlKey = "byPathFn"
)

func Middleware(router *mux.Router) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), requestURLKey, r.URL)
			ctx = context.WithValue(ctx, routerKey, router)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type URL url.URL

func (u *URL) String() string {
	if u == nil {
		return ""
	}
	return ((*url.URL)(u)).String()
}

func (u *URL) SafeURL() templ.SafeURL {
	if u == nil {
		return templ.SafeURL("")
	}
	return templ.URL((*url.URL)(u).String())
}

func (u *URL) WithParams(params map[string]any) *URL {
	query := ((*url.URL)(u)).Query()
	for key, value := range params {
		switch v := value.(type) {
		case bool:
			if v {
				query.Set(key, "") // Add parameter without a value
			} else {
				query.Del(key) // Remove parameter
			}
		case nil:
			query.Del(key) // Remove parameter
		default:
			query.Set(key, fmt.Sprintf("%v", v)) // Convert value to string
		}
	}
	((*url.URL)(u)).RawQuery = query.Encode()
	return u
}

func RequestURL(ctx context.Context) *URL {
	if ctx == nil {
		return nil
	}
	u, ok := ctx.Value(requestURLKey).(*url.URL)
	if !ok || u == nil {
		return nil
	}
	return (*URL)(u)
}

func Router(ctx context.Context) *mux.Router {
	if ctx == nil {
		return mux.NewRouter()
	}
	router, ok := ctx.Value(routerKey).(*mux.Router)
	if !ok || router == nil {
		return mux.NewRouter()
	}
	return router
}

func RouterURL(router *mux.Router, name string, pairs ...string) (string, error) {
	if router == nil {
		return "", fmt.Errorf("route %s not found", name)
	}

	url := router.Get(name)
	if url == nil {
		return "", fmt.Errorf("route %s not found", name)
	}

	u, err := url.URL(pairs...)
	if err != nil {
		return "", fmt.Errorf("failed to generate URL for route %s: %w", name, err)
	}

	return u.String(), nil
}
