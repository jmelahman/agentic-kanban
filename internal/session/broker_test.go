package session

import (
	"bytes"
	"testing"
)

func TestRingBuffer_PartialFill(t *testing.T) {
	r := newRingBuffer(16)
	r.Write([]byte("hello"))
	if got, want := r.Snapshot(), []byte("hello"); !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	r.Write([]byte(" world"))
	if got, want := r.Snapshot(), []byte("hello world"); !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRingBuffer_Wrap(t *testing.T) {
	r := newRingBuffer(8)
	r.Write([]byte("abcdef"))
	r.Write([]byte("ghijk")) // 11 bytes total; last 8 should be retained
	if got, want := r.Snapshot(), []byte("defghijk"); !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRingBuffer_OversizedSingleWrite(t *testing.T) {
	r := newRingBuffer(4)
	r.Write([]byte("0123456789"))
	if got, want := r.Snapshot(), []byte("6789"); !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	// Subsequent small write should append to the latest tail.
	r.Write([]byte("ab"))
	if got, want := r.Snapshot(), []byte("89ab"); !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRingBuffer_ExactSize(t *testing.T) {
	r := newRingBuffer(4)
	r.Write([]byte("abcd"))
	if got, want := r.Snapshot(), []byte("abcd"); !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	r.Write([]byte("ef"))
	if got, want := r.Snapshot(), []byte("cdef"); !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRingBuffer_Empty(t *testing.T) {
	r := newRingBuffer(8)
	if got := r.Snapshot(); len(got) != 0 {
		t.Fatalf("got %q want empty", got)
	}
}
