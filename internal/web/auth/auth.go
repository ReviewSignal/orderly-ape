// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/ReviewSignal/orderly-ape/internal/util/rand"
)

const (
	accessTokenCookie = "access_token"
	idTokenCookie     = "id_token"
)

const (
	AdminGroup = "ape.reviewsignal.com/admin"
	UserGroup  = "ape.reviewsignal.com/user"
)

func (a *Auth) EncryptCookie(cookie http.Cookie) http.Cookie {
	return cookie
}

func (a *Auth) DecryptCookie(cookie http.Cookie) http.Cookie {
	return cookie
}

type Auth struct {
	ClientID          string
	ClientSecret      string
	OAuth2RedirectURL string
	OIDCProvider      *oidc.Provider

	Client              client.Client
	ControllerNamespace string

	callbackPath string
}

func NewAuth(a Auth) *Auth {
	url, err := url.Parse(a.OAuth2RedirectURL)
	a.callbackPath = url.Path
	if err != nil || url.Path == "" {
		a.callbackPath = "/"
	}
	return &a
}

func (a *Auth) oauthStateCookie(state string) http.Cookie {
	return http.Cookie{
		Name:     "oauthstate",
		Path:     a.callbackPath,
		Value:    state,
		MaxAge:   15 * 60, // 15 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	}
}

func generateStateHash(state string) string {
	hash := sha256.New().Sum([]byte(state))
	return hex.EncodeToString(hash)
}

func (a *Auth) OAuth2Config(redirectURL string, scopes []string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     a.ClientID,
		ClientSecret: a.ClientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     a.OIDCProvider.Endpoint(),
		Scopes:       scopes,
	}
}

func (a *Auth) AuthCallback(w http.ResponseWriter, r *http.Request) {
	log := logf.FromContext(r.Context())
	stateHash := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	// clear the state cookie
	oauthStateCookie := a.oauthStateCookie("")
	oauthStateCookie.MaxAge = -1
	http.SetCookie(w, &oauthStateCookie)

	stateCookie, err := r.Cookie("oauthstate")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	// Verify state
	if stateHash == "" || stateHash != generateStateHash(stateCookie.Value) {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	// Exchange code for token
	oauth2 := a.OAuth2Config(a.RedirectURL(r), []string{oidc.ScopeOpenID, "profile", "email"})
	token, err := oauth2.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		log.Error(err, "no id_token in token response")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	verifier := a.OIDCProvider.Verifier(&oidc.Config{ClientID: a.ClientID})
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		log.Error(err, "failed to verify ID token")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	accessToken, ok := token.Extra("access_token").(string)
	if !ok {
		log.Error(err, "no access_token in token response")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// Extract custom claims
	var claims User
	if err := idToken.Claims(&claims); err != nil {
		log.Error(err, "failed to decode ID token claims")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	idTokenCookie := a.EncryptCookie(http.Cookie{
		Name:     idTokenCookie,
		Value:    rawIDToken,
		Path:     "/",
		HttpOnly: true,
		Expires:  idToken.Expiry,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})
	http.SetCookie(w, &idTokenCookie)

	accessTokenCookie := a.EncryptCookie(http.Cookie{
		Name:     accessTokenCookie,
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Expires:  token.Expiry,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})
	http.SetCookie(w, &accessTokenCookie)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *Auth) RequireAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !UserInGroup(r.Context(), AdminGroup) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *Auth) RedirectURL(r *http.Request) string {
	if a.OAuth2RedirectURL != "" {
		return a.OAuth2RedirectURL
	}
	if r == nil || r.URL == nil {
		return "http://localhost:8080/auth/callback"
	}
	url := *r.URL

	url.Host = r.Host
	if url.Scheme == "" && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https") {
		url.Scheme = "https" // Use HTTPS if the request is over TLS
	} else if url.Scheme == "" {
		url.Scheme = "http"
	}
	url.RawPath = "/auth/callback"

	return url.String()
}

func (a *Auth) InjectUserMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := logf.FromContext(r.Context())

		var idToken *oidc.IDToken
		idTokenCookie, err := r.Cookie(idTokenCookie)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		_, err = r.Cookie(accessTokenCookie)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		verifier := a.OIDCProvider.Verifier(&oidc.Config{ClientID: a.ClientID})
		idToken, err = verifier.Verify(r.Context(), idTokenCookie.Value)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		// Extract custom claims
		var claims User
		err = idToken.Claims(&claims)
		if err != nil {
			l.Error(err, "failed to decode ID token claims from cookie")
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), UserKey, &claims)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

func (a *Auth) RequireAuthenticationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(UserKey).(*User)
		if !ok || user == nil || user.Email == "" {
			// Generate a new state token
			state := rand.MustAlphaNumericString(64)
			stateCookie := a.oauthStateCookie(state)

			http.SetCookie(w, &stateCookie)
			stateHash := generateStateHash(state)

			oauth2 := a.OAuth2Config(a.RedirectURL(r), []string{oidc.ScopeOpenID, "profile", "email"})
			authURL := oauth2.AuthCodeURL(stateHash)
			http.Redirect(w, r, authURL, http.StatusSeeOther)
		}

		next.ServeHTTP(w, r)
	})
}

func (a *Auth) LogoutMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:   accessTokenCookie,
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
		http.SetCookie(w, &http.Cookie{
			Name:   idTokenCookie,
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})

		next.ServeHTTP(w, r)
	})
}

func (a *Auth) logoutCallback(w http.ResponseWriter, _ *http.Request) {
	_, _ = fmt.Fprint(w, "Logged out")
}

func (a *Auth) LogoutCallback(w http.ResponseWriter, r *http.Request) {
	a.LogoutMiddleware(http.HandlerFunc(a.logoutCallback)).ServeHTTP(w, r)
}
