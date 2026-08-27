package conversation

import "testing"

func TestParseSessions(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []Live
	}{
		{"empty", "", []Live{}},
		{"garbage", "no server running on /tmp/tmux-1000/default", []Live{}},
		{
			"sessions only",
			"conv-1|0\nconv-2|1\n",
			[]Live{{ID: "1"}, {ID: "2", Attached: true}},
		},
		{
			"bell on unattended session",
			"conv-1|0\nBELL|conv-1|1\n",
			[]Live{{ID: "1", Bell: true}},
		},
		{
			"bell flag zero is not a bell",
			"conv-1|0\nBELL|conv-1|0",
			[]Live{{ID: "1"}},
		},
		{
			"bell on foreign session ignored",
			"conv-1|0\nBELL|scratch|1",
			[]Live{{ID: "1"}},
		},
		{
			"foreign sessions skipped",
			"scratch|1\nconv-3|0",
			[]Live{{ID: "3"}},
		},
		{
			"partial output after list-windows failure",
			"conv-1|1\n",
			[]Live{{ID: "1", Attached: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSessions(tt.out, Prefix)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("session %d: got %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
