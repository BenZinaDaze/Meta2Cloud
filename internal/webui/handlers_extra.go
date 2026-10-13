// handlers_extra.go —— 查询参数工具。
package webui

import (
	"net/http"
	"strconv"

	"meta2cloud/internal/webui/store"
)

func intQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func ingestQueryFromParams(q map[string][]string) store.IngestQuery {
	get := func(key string) string {
		if vals, ok := q[key]; ok && len(vals) > 0 {
			return vals[0]
		}
		return ""
	}
	tmdbID := int64(0)
	if v := get("tmdb_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			tmdbID = n
		}
	}
	return store.IngestQuery{
		Page:      intParam(get("page"), 1),
		PageSize:  intParam(get("page_size"), 20),
		MediaType: get("media_type"),
		TmdbID:    tmdbID,
		Status:    get("status"),
		StartDate: get("start_date"),
		EndDate:   get("end_date"),
		Keyword:   get("keyword"),
	}
}

func intParam(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
