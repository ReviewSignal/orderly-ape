// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package utils

import (
	"iter"
	"net/http"
	"slices"
	"strconv"
)

// PaginationURL generates a URL for a specific page.
func PaginationURL(baseURL string, paginationKey string, page int) string {
	if page <= 1 {
		return SetQueryParams(baseURL, map[string]any{paginationKey: nil})
	}

	return SetQueryParams(baseURL, map[string]any{paginationKey: page})
}

func CurrentPage(r *http.Request, pageArgument ...string) int {
	pageArg := "page"
	if len(pageArgument) > 0 {
		pageArg = pageArgument[0]
	}
	if pageArg == "" {
		pageArg = "page"
	}

	if r == nil || r.URL == nil || r.URL.Query() == nil || r.URL.Query().Get(pageArg) == "" {
		return 0
	}

	currentPage, err := strconv.Atoi(r.URL.Query().Get(pageArg))
	if err != nil || currentPage < 1 {
		currentPage = 1
	}

	return currentPage
}

func Paginate[K any](items []K, perPage int, currentPage int) (iter.Seq[K], int, int) {
	totalPages := len(items) / perPage
	if len(items)%perPage != 0 {
		totalPages++
	}

	if currentPage > totalPages {
		currentPage = totalPages
	}

	if currentPage < 1 {
		currentPage = 1
	}

	var paginated []K
	if currentPage >= totalPages { // last page
		paginated = items[(currentPage-1)*perPage:]
	} else {
		paginated = items[(currentPage-1)*perPage : currentPage*perPage]
	}

	return slices.Values(paginated), totalPages, len(paginated)
}

func PaginateRequest[K any](r *http.Request, items []K, perPage int) (iter.Seq[K], int, int, int) {
	currentPage := CurrentPage(r)
	paginated, totalPages, numItems := Paginate(items, perPage, currentPage)
	if currentPage > totalPages {
		currentPage = totalPages
	}

	return paginated, totalPages, currentPage, numItems
}
