package githubrelease

import "testing"

func TestTagVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tag  string
		want string
	}{
		{tag: "v1.2.3", want: "1.2.3"},
		{tag: "V1.2.3", want: "1.2.3"},
		{tag: "1.2.3", want: "1.2.3"},
	}
	for _, tt := range tests {
		if got := TagVersion(tt.tag); got != tt.want {
			t.Fatalf("TagVersion(%q) = %q, want %q", tt.tag, got, tt.want)
		}
	}
}

func TestDownloadBaseURL(t *testing.T) {
	t.Parallel()
	in := "https://github.com/pulseaiclub/phi/releases/tag/v0.1.0"
	want := "https://github.com/pulseaiclub/phi/releases/download/v0.1.0"
	if got := DownloadBaseURL(in); got != want {
		t.Fatalf("DownloadBaseURL() = %q, want %q", got, want)
	}
}
