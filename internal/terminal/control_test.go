package terminal

import (
	"bytes"
	"errors"
	"testing"
)

// classifyControl (ADR-079): a ping is answered and is NOT activity; a resize
// keeps its size and is activity; anything else is ignored and still counts
// as activity, as every client frame did before.
func TestClassifyControl(t *testing.T) {
	cases := []struct {
		name         string
		in           string
		wantOK       bool
		wantType     string
		wantReply    string // "" = no reply
		wantActivity bool
		cols, rows   uint16
	}{
		{"ping is answered and is not activity", `{"type":"ping"}`, true, "ping", `{"type":"pong"}`, false, 0, 0},
		{"resize keeps its size", `{"type":"resize","cols":120,"rows":30}`, true, "resize", "", true, 120, 30},
		{"unknown type is ignored", `{"type":"pong"}`, false, "", "", true, 0, 0},
		{"not JSON is ignored", `ping`, false, "", "", true, 0, 0},
		{"empty frame is ignored", ``, false, "", "", true, 0, 0},
		{"a JSON array is ignored", `[{"type":"ping"}]`, false, "", "", true, 0, 0},
		{"type must be a string", `{"type":1}`, false, "", "", true, 0, 0},
	}
	for _, c := range cases {
		msg, ok, reply, activity := classifyControl([]byte(c.in))
		if ok != c.wantOK || msg.Type != c.wantType {
			t.Errorf("%s: ok=%v type=%q, want ok=%v type=%q", c.name, ok, msg.Type, c.wantOK, c.wantType)
		}
		if c.wantReply == "" {
			if reply != nil {
				t.Errorf("%s: reply %q, want none", c.name, reply)
			}
		} else if !bytes.Equal(reply, []byte(c.wantReply)) {
			t.Errorf("%s: reply %q, want %q", c.name, reply, c.wantReply)
		}
		if activity != c.wantActivity {
			t.Errorf("%s: activity=%v, want %v", c.name, activity, c.wantActivity)
		}
		if msg.Cols != c.cols || msg.Rows != c.rows {
			t.Errorf("%s: size %dx%d, want %dx%d", c.name, msg.Cols, msg.Rows, c.cols, c.rows)
		}
	}
}

// applyControl is the read loop's whole handling of a text frame. The case
// that matters most: a ping must never call touch — a visible tab pings every
// 25 s, and counting that as use would keep its project from the idle reaper
// for as long as the tab stays open.
func TestApplyControl(t *testing.T) {
	type size struct{ cols, rows uint }
	cases := []struct {
		name        string
		in          string
		wantTouches int
		wantResize  []size
		wantReplies []string
	}{
		{"ping: answered, not activity", `{"type":"ping"}`, 0, nil, []string{`{"type":"pong"}`}},
		{"resize: activity, resized", `{"type":"resize","cols":120,"rows":30}`, 1, []size{{120, 30}}, nil},
		{"resize without a size: activity, not resized", `{"type":"resize","cols":0,"rows":30}`, 1, nil, nil},
		{"unknown frame: activity, nothing else", `{"type":"pong"}`, 1, nil, nil},
		{"garbage: activity, nothing else", `not json`, 1, nil, nil},
	}
	for _, c := range cases {
		touches := 0
		var resized []size
		var replies []string
		err := applyControl([]byte(c.in),
			func() { touches++ },
			func(cols, rows uint) { resized = append(resized, size{cols, rows}) },
			func(frame []byte) error { replies = append(replies, string(frame)); return nil })
		if err != nil {
			t.Errorf("%s: err = %v", c.name, err)
		}
		if touches != c.wantTouches {
			t.Errorf("%s: touch called %d times, want %d", c.name, touches, c.wantTouches)
		}
		if len(resized) != len(c.wantResize) || (len(resized) == 1 && resized[0] != c.wantResize[0]) {
			t.Errorf("%s: resized %v, want %v", c.name, resized, c.wantResize)
		}
		if len(replies) != len(c.wantReplies) || (len(replies) == 1 && replies[0] != c.wantReplies[0]) {
			t.Errorf("%s: replies %v, want %v", c.name, replies, c.wantReplies)
		}
	}
}

// A reply that cannot be written ends the attachment: the error must reach
// the read loop, not vanish.
func TestApplyControlReturnsTheReplyError(t *testing.T) {
	want := errors.New("peer stopped reading")
	err := applyControl([]byte(`{"type":"ping"}`), func() {}, func(uint, uint) {}, func([]byte) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want the reply's write error", err)
	}
}
