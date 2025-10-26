// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package utils

import "context"

type ctxVar string

var titleKey = ctxVar("title")

func GetPageTitle(ctx context.Context) string {
	title, ok := ctx.Value(titleKey).(string)
	if !ok {
		return ""
	}
	return title
}

func SetPageTitle(ctx context.Context, title string) context.Context {
	return context.WithValue(ctx, titleKey, title)
}
