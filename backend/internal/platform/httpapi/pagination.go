package httpapi

import (
	"net/url"
	"strconv"
	"time"
)

const (
	defaultPageLimit = 20
	maximumPageLimit = 100
	maximumCursorLen = 512
)

type PageRequest struct {
	Limit  int
	Cursor string
}

type PageMeta struct {
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func ParsePage(values url.Values) (PageRequest, *Error) {
	page := PageRequest{Limit: defaultPageLimit}
	if limits, exists := values["limit"]; exists {
		if len(limits) != 1 {
			return PageRequest{}, InvalidRequest("limit must be provided once")
		}
		limit, err := strconv.Atoi(limits[0])
		if err != nil || limit < 1 || limit > maximumPageLimit {
			return PageRequest{}, InvalidRequest("limit must be an integer between 1 and 100")
		}
		page.Limit = limit
	}
	if cursors, exists := values["cursor"]; exists {
		if len(cursors) != 1 || len(cursors[0]) > maximumCursorLen {
			return PageRequest{}, InvalidRequest("cursor is invalid")
		}
		page.Cursor = cursors[0]
	}
	return page, nil
}

func FormatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
