package router

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func TestBuildQueryParams(t *testing.T) {
	u, err := url.Parse("http://100.64.0.7:3100/api/js?data=123&tag=a&tag=b&empty=")
	if err != nil {
		t.Fatalf("parse url failed: %v", err)
	}

	got := buildQueryParams(u)
	want := map[string]string{
		"data":  "123",
		"tag":   "b",
		"empty": "",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected params, got=%v want=%v", got, want)
	}
}

func TestBuildQueryParamsNilURL(t *testing.T) {
	got := buildQueryParams(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty params map, got=%v", got)
	}
}

func TestShouldStripRawBody(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{
			name:        "multipart带boundary",
			contentType: "multipart/form-data; boundary=----WebKitFormBoundaryabc",
			want:        true,
		},
		{
			name:        "multipart无boundary也应识别",
			contentType: "multipart/form-data",
			want:        true,
		},
		{
			name:        "json不应清空body",
			contentType: "application/json",
			want:        false,
		},
		{
			name:        "空content-type不应清空body",
			contentType: "",
			want:        false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := shouldStripRawBody(tc.contentType)
			if got != tc.want {
				t.Fatalf("unexpected shouldStripRawBody result, got=%v want=%v contentType=%q", got, tc.want, tc.contentType)
			}
		})
	}
}

func TestBuildFullURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/tea/123?x=1", nil)
	got := buildFullURL(req)
	want := "http://example.com/tea/123?x=1"
	if got != want {
		t.Fatalf("unexpected full url, got=%s want=%s", got, want)
	}
}

func TestBuildFullURLForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal.local/tea/123?x=1", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "api.example.com")

	got := buildFullURL(req)
	want := "https://api.example.com/tea/123?x=1"
	if got != want {
		t.Fatalf("unexpected full url with forwarded headers, got=%s want=%s", got, want)
	}
}

func TestBuildRouteSuffix(t *testing.T) {
	tests := []struct {
		name        string
		route       string
		requestPath string
		want        string
	}{
		{
			name:        "基础路径无斜杠",
			route:       "/tea/*",
			requestPath: "/tea",
			want:        "/",
		},
		{
			name:        "基础路径带斜杠",
			route:       "/tea/*",
			requestPath: "/tea/",
			want:        "/",
		},
		{
			name:        "子路径",
			route:       "/tea/*",
			requestPath: "/tea/a",
			want:        "/a",
		},
		{
			name:        "基础路径带查询串",
			route:       "/tea/*",
			requestPath: "/tea/?token=1",
			want:        "/",
		},
		{
			name:        "子路径带查询串",
			route:       "/tea/*",
			requestPath: "/tea/a?token=1",
			want:        "/a",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := buildRouteSuffix(tc.route, tc.requestPath)
			if got != tc.want {
				t.Fatalf("unexpected route suffix, got=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestRequestPathDoesNotIncludeQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tea?token=1", nil)
	if req.URL.Path != "/tea" {
		t.Fatalf("unexpected request path, got=%q want=%q", req.URL.Path, "/tea")
	}

	params := buildQueryParams(req.URL)
	want := map[string]string{"token": "1"}
	if !reflect.DeepEqual(params, want) {
		t.Fatalf("unexpected query params, got=%v want=%v", params, want)
	}
}
