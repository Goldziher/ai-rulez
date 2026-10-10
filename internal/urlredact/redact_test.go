package urlredact

import "testing"

func TestHasCredentials(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"userinfo token", "https://user:token@host/org/repo.git", true},
		{"userinfo only", "https://token@host/org/repo.git", true},
		{"git+https userinfo", "git+https://user:token@host/org/repo.git", true},
		{"query access_token", "https://host/org/repo.git?access_token=abc", true},
		{"query private_token", "https://host/repo?private_token=abc&x=1", true},
		{"query api_key", "https://host/repo?api_key=abc", true},
		{"plain https", "https://host/org/repo.git", false},
		{"plain https with ref query", "https://host/org/repo.git?ref=main", false},
		{"ssh user is not a credential", "ssh://git@host/org/repo.git", false},
		{"scp style", "git@host:org/repo.git", false},
		{"file", "file:///srv/repo", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasCredentials(tt.url); got != tt.want {
				t.Errorf("HasCredentials(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestURLRedactsUserinfoAndQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"userinfo", "https://user:s3cret@host/org/repo.git", "https://<redacted>@host/org/repo.git"},
		{"query values", "https://host/repo?access_token=s3cret&ref=main", "https://host/repo?access_token=<redacted>&ref=<redacted>"},
		{"clean", "https://host/org/repo.git", "https://host/org/repo.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := URL(tt.in); got != tt.want {
				t.Errorf("URL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHasUserinfo(t *testing.T) {
	if !HasUserinfo("url = https://user:token@host/x") {
		t.Error("HasUserinfo did not find userinfo")
	}
	if HasUserinfo("url = https://host/x") {
		t.Error("HasUserinfo found userinfo in a clean URL")
	}
}
