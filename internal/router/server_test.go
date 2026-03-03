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
	got := req.URL.String()
	want := "http://example.com/tea/123?x=1"
	t.Logf("%s", got)
	if got != want {
		t.Fatalf("unexpected full url, got=%s want=%s", got, want)
	}
}

func TestBuildFullURLForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal.local/tea/123?x=1", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "api.example.com")

	got := req.URL.String()
	want := "https://api.example.com/tea/123?x=1"
	if got != want {
		t.Fatalf("unexpected full url with forwarded headers, got=%s want=%s", got, want)
	}
}
