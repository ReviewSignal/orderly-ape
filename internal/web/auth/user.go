// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"

	lru "github.com/hashicorp/golang-lru/v2"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type contextKey string

var UserKey = contextKey("user")

type stringAsBool bool

var gravatarCache *lru.Cache[string, []byte]

func init() {
	var err error
	gravatarCache, err = lru.New[string, []byte](100)
	if err != nil {
		panic(err)
	}
}

func (sb *stringAsBool) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case "true", `"true"`:
		*sb = true
	case "false", `"false"`:
		*sb = false
	default:
		return errors.New("invalid value for boolean")
	}
	return nil
}

type User struct {
	Subject string `json:"sub"`
	Profile string `json:"profile"`
	Email   string `json:"email"`
	// Handle providers that return email_verified as a string
	// https://forums.aws.amazon.com/thread.jspa?messageID=949441&#949441 and
	// https://discuss.elastic.co/t/openid-error-after-authenticating-against-aws-cognito/206018/11
	EmailVerified stringAsBool `json:"email_verified"`
	FirstName     string       `json:"given_name"`
	LastName      string       `json:"family_name"`
	Picture       string       `json:"picture"`
	Groups        []string     `json:"groups,omitempty"`
}

func GetUser(ctx context.Context) *User {
	if user, ok := ctx.Value(UserKey).(*User); ok {
		return user
	}
	return &User{}
}

func GetEmail(ctx context.Context) string {
	return GetUser(ctx).Email
}

func GetGroups(ctx context.Context) []string {
	return GetUser(ctx).Groups
}

func UserIsAuthenticated(ctx context.Context) bool {
	return GetEmail(ctx) != ""
}

func UserInGroup(ctx context.Context, group string) bool {
	return slices.Contains(GetGroups(ctx), group)
}

func GetFullName(ctx context.Context) string {
	user := GetUser(ctx)
	return fmt.Sprintf("%s %s", user.FirstName, user.LastName)
}

func GetPicture(ctx context.Context) string {
	picture := GetUser(ctx).Picture
	if picture == "" && GetEmail(ctx) != "" {
		hash := sha256.Sum256([]byte(GetEmail(ctx)))
		return fmt.Sprintf("https://www.gravatar.com/avatar/%x", hash)
	}
	return picture
}

func UserAvatar(w http.ResponseWriter, r *http.Request) {
	var found bool
	var pictureData []byte
	user := GetUser(r.Context())
	pictureURL := GetPicture(r.Context())

	log := log.FromContext(r.Context())
	// Check if the picture is already cached
	if pictureData, found = gravatarCache.Get(user.Email); !found {
		// Fetch the picture from the URL
		resp, err := http.Get(pictureURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil && resp.StatusCode != http.StatusOK {
				log.Error(err, "failed to fetch picture", "status", resp.StatusCode)
			}
			http.NotFound(w, r)
			return
		}
		defer func() {
			err := resp.Body.Close()
			if err != nil {
				log.Error(err, "failed to close response body")
			}
		}()

		// Read the picture data
		pictureData, err = io.ReadAll(resp.Body)

		// Cache the picture data
		gravatarCache.Add(user.Email, pictureData)

		if err != nil {
			http.NotFound(w, r)
			return
		}
	}

	// Detect MIME type
	mimeType := http.DetectContentType(pictureData)

	// Serve the picture
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pictureData)))
	w.Header().Set("Content-Type", mimeType)
	_, _ = w.Write(pictureData)
}
