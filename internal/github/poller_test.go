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
	if got := APIBaseFor("github.com"); got != defaultAPIBase {
		t.Errorf("github.com: got %q, want %q", got, defaultAPIBase)
	}
	if got := APIBaseFor(""); got != defaultAPIBase {
		t.Errorf("empty host: got %q, want %q", got, defaultAPIBase)
	}
	if got := APIBaseFor("ghe.example.com"); got != "https://ghe.example.com/api/v3" {
		t.Errorf("GHE host: got %q", got)
	}
	t.Setenv("GITHUB_API_URL", "https://override.example/api/")
	if got := APIBaseFor("github.com"); got != "https://override.example/api" {
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
		if got := ParseNextLink(c.in); got != c.want {
			t.Errorf("ParseNextLink(%q) = %q, want %q", c.in, got, c.want)
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
	resetTokenCache()
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "fallback")
	if got := Token(""); got != "fallback" {
		t.Errorf("fallback: got %q", got)
	}
	t.Setenv("GH_TOKEN", "primary")
	if got := Token(""); got != "primary" {
		t.Errorf("primary: got %q", got)
	}
}

func TestTokenFallsBackToGHCLI(t *testing.T) {
	resetTokenCache()
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	origLook, origRun := ghTokenLookPath, ghTokenRun
	t.Cleanup(func() {
		ghTokenLookPath, ghTokenRun = origLook, origRun
		resetTokenCache()
	})

	ghTokenLookPath = func(string) (string, error) { return "/usr/bin/gh", nil }
	calls := 0
	var lastArgs []string
	ghTokenRun = func(name string, args ...string) ([]byte, error) {
		calls++
		lastArgs = args
		return []byte("ghcli-token\n"), nil
	}

	if got := Token("github.com"); got != "ghcli-token" {
		t.Errorf("github.com: got %q", got)
	}
	if got := Token("github.com"); got != "ghcli-token" {
		t.Errorf("cached: got %q", got)
	}
	if calls != 1 {
		t.Errorf("expected 1 gh exec, got %d", calls)
	}

	if got := Token("ghe.example.com"); got != "ghcli-token" {
		t.Errorf("ghe: got %q", got)
	}
	if calls != 2 {
		t.Errorf("expected 2 gh execs after ghe lookup, got %d", calls)
	}
	want := []string{"auth", "token", "--hostname", "ghe.example.com"}
	if len(lastArgs) != len(want) {
		t.Fatalf("ghe args: got %v, want %v", lastArgs, want)
	}
	for i := range want {
		if lastArgs[i] != want[i] {
			t.Fatalf("ghe args: got %v, want %v", lastArgs, want)
		}
	}
}

func resetTokenCache() {
	ghTokenCache.Range(func(k, _ any) bool {
		ghTokenCache.Delete(k)
		return true
	})
}
