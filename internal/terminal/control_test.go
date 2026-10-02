package terminal

import (
	"bytes"
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
