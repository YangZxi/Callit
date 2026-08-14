package i18n

import (
	"net/http/httptest"
	"testing"
)

func TestLanguageFromRequest(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"zh-CN,zh;q=0.9", "zh-CN"},
		{"zh-TW", "zh-CN"},
		{"en-US,en;q=0.9", "en"},
		{"en-US,zh-CN;q=0.9", "en"},
		{"fr-FR", "en"},
	}
	for _, test := range tests {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Language", test.header)
		if got := LanguageFromRequest(req); got != test.want {
			t.Fatalf("header %q: got %q, want %q", test.header, got, test.want)
		}
	}
}

func TestLocalizeLegacyMessage(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"zh-CN", "未授权"},
		{"en-US", "Unauthorized"},
	}
	for _, test := range tests {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Language", test.header)
		if got := LocalizeLegacyMessage(req, "unauthorized"); got != test.want {
			t.Fatalf("header %q: got %q, want %q", test.header, got, test.want)
		}
	}
}
