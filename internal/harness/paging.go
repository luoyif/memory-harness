package harness

import "strings"

type ObjectListOptions struct {
	ProjectID            string
	TypeID               string
	Status               string
	Limit                int
	Offset               int
	ExcludedTypeIDs      []string
	ExcludedTypePrefixes []string
}

type PageInfo struct {
	Total   int  `json:"total"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

func normalizePage(limit, offset int) (int, int) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func cleanPrefix(value string) string { return strings.TrimSpace(value) }
