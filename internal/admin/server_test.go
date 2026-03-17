package admin

import "testing"

func TestIsAdminAPIPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/admin/api/auth/status", want: true},
		{path: "/admin/api/workers", want: true},
		{path: "/admin", want: false},
		{path: "/api/auth/status", want: false},
	}

	for _, tc := range tests {
		got := isAdminAPIPath(tc.path, "/admin")
		if got != tc.want {
			t.Fatalf("unexpected admin api path result for %q: got=%v want=%v", tc.path, got, tc.want)
		}
	}
}

func TestPublicFilePathFromAdminPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/admin", want: "public"},
		{path: "/admin/", want: "public"},
		{path: "/admin/assets/app.js", want: "public/assets/app.js"},
		{path: "/admin/workers/123", want: "public/workers/123"},
	}

	for _, tc := range tests {
		got := publicFilePathFromAdminPath(tc.path, "/admin")
		if got != tc.want {
			t.Fatalf("unexpected public file path for %q: got=%q want=%q", tc.path, got, tc.want)
		}
	}
}
