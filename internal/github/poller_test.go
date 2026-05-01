package github

import "testing"

func TestParseRemoteURL(t *testing.T) {
	cases := []struct {
		in    string
		owner string
		repo  string
		host  string
		bad   bool
	}{
		{in: "git@github.com:owner/repo.git", owner: "owner", repo: "repo", host: "github.com"},
		{in: "git@github.com:owner/repo", owner: "owner", repo: "repo", host: "github.com"},
		{in: "ssh://git@github.com/owner/repo.git", owner: "owner", repo: "repo", host: "github.com"},
		{in: "https://github.com/owner/repo.git", owner: "owner", repo: "repo", host: "github.com"},
		{in: "https://github.com/owner/repo", owner: "owner", repo: "repo", host: "github.com"},
		{in: "git@ghe.example.com:team/proj.git", owner: "team", repo: "proj", host: "ghe.example.com"},
		{in: "https://ghe.example.com/team/proj", owner: "team", repo: "proj", host: "ghe.example.com"},
		{in: "git@github.com:owner", bad: true},
		{in: "https://github.com/onlyone", bad: true},
		{in: "not a url", bad: true},
	}
	for _, c := range cases {
		owner, repo, host, err := parseRemoteURL(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("parseRemoteURL(%q): want error, got %s/%s/%s", c.in, owner, repo, host)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseRemoteURL(%q): unexpected error: %v", c.in, err)
			continue
		}
		if owner != c.owner || repo != c.repo || host != c.host {
			t.Errorf("parseRemoteURL(%q): got %s/%s@%s, want %s/%s@%s",
				c.in, owner, repo, host, c.owner, c.repo, c.host)
		}
	}
}

func TestApiBaseFor(t *testing.T) {
	t.Setenv("GITHUB_API_URL", "")
	if got := apiBaseFor("github.com"); got != defaultAPIBase {
		t.Errorf("github.com: got %q, want %q", got, defaultAPIBase)
	}
	if got := apiBaseFor(""); got != defaultAPIBase {
		t.Errorf("empty host: got %q, want %q", got, defaultAPIBase)
	}
	if got := apiBaseFor("ghe.example.com"); got != "https://ghe.example.com/api/v3" {
		t.Errorf("GHE host: got %q", got)
	}
	t.Setenv("GITHUB_API_URL", "https://override.example/api/")
	if got := apiBaseFor("github.com"); got != "https://override.example/api" {
		t.Errorf("override: got %q", got)
	}
}

func TestParseNextLink(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{
			in:   `<https://api.github.com/repositories/1/pulls?page=2>; rel="next", <https://api.github.com/repositories/1/pulls?page=5>; rel="last"`,
			want: "https://api.github.com/repositories/1/pulls?page=2",
		},
		{
			in:   `<https://api.github.com/repositories/1/pulls?page=5>; rel="last"`,
			want: "",
		},
	}
	for _, c := range cases {
		if got := parseNextLink(c.in); got != c.want {
			t.Errorf("parseNextLink(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClassify(t *testing.T) {
	merged := "2024-01-01T00:00:00Z"
	cases := []struct {
		name string
		pr   ghPR
		want string
	}{
		{name: "open", pr: ghPR{State: "open"}, want: PRStateOpen},
		{name: "open draft", pr: ghPR{State: "open", Draft: true}, want: PRStateDraft},
		{name: "closed unmerged", pr: ghPR{State: "closed"}, want: PRStateClosed},
		{name: "closed merged", pr: ghPR{State: "closed", MergedAt: &merged}, want: PRStateMerged},
		{name: "unknown", pr: ghPR{State: ""}, want: ""},
	}
	for _, c := range cases {
		if got := classify(c.pr); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTokenPrefersGHToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "fallback")
	if got := token(); got != "fallback" {
		t.Errorf("fallback: got %q", got)
	}
	t.Setenv("GH_TOKEN", "primary")
	if got := token(); got != "primary" {
		t.Errorf("primary: got %q", got)
	}
}
