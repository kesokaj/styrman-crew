package orchestrator

import (
	"strings"
	"testing"
)

// TestTailBufferBoundsHostileOutput is the regression guard for the audit's
// high finding: container logs were buffered into an unbounded strings.Builder
// under a comment claiming `Tail` made it safe. `Tail` bounds LINES, not bytes,
// and the guest is root in its own VM and picks the line length — so one
// project could drive the SHARED server out of memory.
func TestTailBufferBoundsHostileOutput(t *testing.T) {
	const max = 64 << 10
	b := &tailBuffer{max: max}

	// A hostile guest: 512 writes of 1 MiB each = 512 MiB streamed through a
	// buffer that must never hold more than `max`.
	chunk := strings.Repeat("A", 1<<20)
	for i := 0; i < 512; i++ {
		n, err := b.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("Write must accept everything so the reader keeps draining: n=%d err=%v", n, err)
		}
		if len(b.buf) > max {
			t.Fatalf("buffer grew past the cap: %d > %d", len(b.buf), max)
		}
		if cap(b.buf) > 4*max {
			t.Fatalf("underlying array grew unbounded: cap=%d (a reslice leak)", cap(b.buf))
		}
	}
	if !b.Dropped() {
		t.Error("Dropped() must report that the front was discarded")
	}
	if got := len(b.String()); got != max {
		t.Errorf("String() length = %d, want the full window %d", got, max)
	}
}

// The whole point of a tail buffer: keep the NEWEST output. cappedBuffer keeps
// the oldest, which a 2026-08-30 review established is wrong for a logs view.
func TestTailBufferKeepsTheNewest(t *testing.T) {
	b := &tailBuffer{max: 10}
	for _, s := range []string{"oldest---", "middle---", "NEWEST"} {
		_, _ = b.Write([]byte(s))
	}
	got := b.String()
	if !strings.HasSuffix(got, "NEWEST") {
		t.Errorf("tail lost: %q does not end with the last write", got)
	}
	if strings.Contains(got, "oldest") {
		t.Errorf("tail kept the oldest bytes: %q", got)
	}
	if len(got) != 10 {
		t.Errorf("length = %d, want 10", len(got))
	}
}

// Review finding, mutation-proven gap: a single write of EXACTLY max bytes
// into an empty buffer discards nothing and must not claim truncation — the
// spurious flag cost a real first line plus a false banner at the call site.
func TestTailBufferExactMaxSingleWriteIsNotTruncation(t *testing.T) {
	b := &tailBuffer{max: 8}
	_, _ = b.Write([]byte("12345678"))
	if b.Dropped() {
		t.Error("exactly-max write into an empty buffer discards nothing — Dropped() must be false")
	}
	if b.String() != "12345678" {
		t.Errorf("content altered: %q", b.String())
	}
	if !b.StartsAtLineBoundary() {
		t.Error("nothing dropped — the window starts at a line boundary by definition")
	}
}

func TestTailBufferUnderCap(t *testing.T) {
	b := &tailBuffer{max: 1024}
	_, _ = b.Write([]byte("short output\n"))
	if b.Dropped() {
		t.Error("Dropped() must be false when nothing was discarded")
	}
	if b.String() != "short output\n" {
		t.Errorf("content altered under the cap: %q", b.String())
	}
}

// trimLogTail is where an out-of-range index (out[len(out)-max-1] == -1 when
// the buffer is full) panicked on the hostile input the cap was added for.
// These pin every shape that reaches it.
func TestTrimLogTail(t *testing.T) {
	const banner = "… (older lines truncated) …\n"
	cases := []struct {
		name     string
		out      string
		dropped  bool
		boundary bool
		want     string
	}{
		{"nothing dropped is returned verbatim", "a\nb\nc", false, true, "a\nb\nc"},
		{"empty, nothing dropped", "", false, true, ""},
		{"empty, dropped, boundary", "", true, true, banner},
		{"dropped mid-line strips the fragment", "rtial\nb\nc", true, false, banner + "b\nc"},
		{"dropped on a boundary keeps the first line", "a\nb\nc", true, true, banner + "a\nb\nc"},
		// No newline to strip to: the window is one long line-tail. Keep it
		// after the banner rather than blanking it — the tail is what a tail
		// view exists to show, and the banner already signals truncation.
		{"dropped, no newline, keeps the line-tail", "noNewlineFragment", true, false, banner + "noNewlineFragment"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := trimLogTail(c.out, c.dropped, c.boundary); got != c.want {
				t.Errorf("trimLogTail(%q,%v,%v) = %q, want %q", c.out, c.dropped, c.boundary, got, c.want)
			}
		})
	}
}

// The boundary flag must come from tailBuffer, so exercise the two together:
// a discard that lands exactly on a newline must NOT eat the next full line
// (the bug the review caught after cutMidLine was removed).
func TestTailBufferBoundaryDoesNotEatALine(t *testing.T) {
	b := &tailBuffer{max: 6}
	// Stream "aaa\nbbb" through a 6-byte window. The window ends up "\nbbb"...
	// arrange so the discard lands right after the first '\n'.
	_, _ = b.Write([]byte("aaa\n")) // buf = "aaa\n"
	_, _ = b.Write([]byte("bbb"))   // len 4+3=7 > 6, drop 1 ("a"), buf="aa\nbbb"
	if !b.Dropped() {
		t.Fatal("expected a drop")
	}
	// Last dropped byte was 'a', not '\n' → not a boundary → first (partial)
	// line "aa" should be stripped, leaving the banner + "bbb".
	got := trimLogTail(b.String(), b.Dropped(), b.StartsAtLineBoundary())
	if got != "… (older lines truncated) …\nbbb" {
		t.Errorf("got %q", got)
	}

	// Now a clean boundary: discard ends exactly on '\n'.
	b2 := &tailBuffer{max: 3}
	_, _ = b2.Write([]byte("x\n")) // buf "x\n"
	_, _ = b2.Write([]byte("yz"))  // 2+2=4>3, drop 1 ("x"), buf "\nyz"
	// lastDrop 'x' → not boundary; but check the exact-newline case explicitly:
	b3 := &tailBuffer{max: 2}
	_, _ = b3.Write([]byte("a\n")) // "a\n"
	_, _ = b3.Write([]byte("bc"))  // 2+2=4>2, drop 2 ("a\n"), buf "bc"; lastDrop '\n'
	if !b3.StartsAtLineBoundary() {
		t.Error("discard ending on newline must be a line boundary")
	}
	if got := trimLogTail(b3.String(), b3.Dropped(), b3.StartsAtLineBoundary()); got != "… (older lines truncated) …\nbc" {
		t.Errorf("boundary case ate a line: %q", got)
	}
}
