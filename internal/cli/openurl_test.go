package cli

import "testing"

func TestWebUIURL(t *testing.T) {
	tests := []struct {
		listen string
		want   string
	}{
		{"127.0.0.1:4680", "http://127.0.0.1:4680"},
		{"0.0.0.0:4680", "http://127.0.0.1:4680"},
		{":4680", "http://127.0.0.1:4680"},
	}
	for _, tc := range tests {
		if got := WebUIURL(tc.listen); got != tc.want {
			t.Fatalf("WebUIURL(%q) = %q, want %q", tc.listen, got, tc.want)
		}
	}
}
