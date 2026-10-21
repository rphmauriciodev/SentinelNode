package web

import (
	_ "embed"
	"net/http"
)

//go:embed dashboard.html
var DashboardHTML []byte

func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(DashboardHTML)
	}
}
