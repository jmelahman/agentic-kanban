package api

import (
	"strings"
	"testing"
)

func TestSlugifyTruncatesLongInput(t *testing.T) {
	long := strings.Repeat("a", 200)
	got := slugify(long)
	if len(got) > slugMaxLen {
		t.Fatalf("slugify(%q) len = %d, want <= %d", long, len(got), slugMaxLen)
	}
}

func TestSlugifyTrimsTrailingDashAfterTruncation(t *testing.T) {
	// 80th char would be a dash; truncation must strip it so the slug is a
	// valid git ref segment.
	in := strings.Repeat("a", slugMaxLen-1) + " bcd"
	got := slugify(in)
	if strings.HasSuffix(got, "-") {
		t.Fatalf("slugify(%q) = %q, has trailing dash", in, got)
	}
	if len(got) > slugMaxLen {
		t.Fatalf("slugify(%q) len = %d, want <= %d", in, len(got), slugMaxLen)
	}
}

func TestSlugifyShortInputUnchanged(t *testing.T) {
	if got, want := slugify("Hello World"), "hello-world"; got != want {
		t.Fatalf("slugify = %q, want %q", got, want)
	}
}
