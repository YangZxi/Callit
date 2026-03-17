package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsAdminPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/admin", want: true},
		{path: "/admin/", want: true},
		{path: "/admin/api/auth/status", want: true},
		{path: "/hello", want: false},
		{path: "/adminx", want: false},
	}

	for _, tc := range tests {
		got := isAdminPath(tc.path, "/admin")
		if got != tc.want {
			t.Fatalf("unexpected admin path result for %q: got=%v want=%v", tc.path, got, tc.want)
		}
	}
}

func TestNewUnifiedHandlerRoutesByPath(t *testing.T) {
	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("admin:" + r.URL.Path))
	})
	routerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("router:" + r.URL.Path))
	})

	handler := serverRouteHandler(adminHandler, routerHandler, "/admin")

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "admin根路径", path: "/admin", want: "admin:/admin"},
		{name: "admin接口路径", path: "/admin/api/auth/status", want: "admin:/admin/api/auth/status"},
		{name: "router普通路径", path: "/hello", want: "router:/hello"},
		{name: "避免误判admin前缀", path: "/adminx", want: "router:/adminx"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		resp := rec.Result()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("%s: read body failed: %v", tc.name, err)
		}
		if string(body) != tc.want {
			t.Fatalf("%s: unexpected body, got=%q want=%q", tc.name, string(body), tc.want)
		}
	}
}
