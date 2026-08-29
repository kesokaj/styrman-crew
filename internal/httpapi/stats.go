package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/orchestrator"
)

// projectStatsView is the live resource snapshot behind the terminal status bar
// (CPU, memory, disk, processes, IP, uptime). Sampled inside the guest.
type projectStatsView struct {
	State      string  `json:"state"`
	CPUPercent int     `json:"cpuPercent"`
	CPUCount   int     `json:"cpuCount"`
	Load1      float64 `json:"load1"`
	MemUsed    int64   `json:"memUsedBytes"`
	MemTotal   int64   `json:"memTotalBytes"`
	DiskUsed   int64   `json:"diskUsedBytes"`
	DiskTotal  int64   `json:"diskTotalBytes"`
	Processes  int     `json:"processes"`
	IP         string  `json:"ip,omitempty"`
	Uptime     int64   `json:"uptimeSeconds"`
	Host       string  `json:"host,omitempty"`
}

// statsScript samples the guest in ONE pass and prints key=value lines. Run
// with a NON-login shell (bash -c, not -lc): a login shell would fire the
// per-login credential pull on every poll. CPU% comes from two /proc/stat
// samples 0.2s apart; everything else is a single read.
const statsScript = `set +e
read -r _ u1 n1 s1 i1 w1 q1 x1 t1 _ < /proc/stat
sleep 0.2
read -r _ u2 n2 s2 i2 w2 q2 x2 t2 _ < /proc/stat
b1=$((u1+n1+s1+q1+x1+t1)); b2=$((u2+n2+s2+q2+x2+t2))
o1=$((b1+i1+w1)); o2=$((b2+i2+w2))
d=$((o2-o1)); db=$((b2-b1)); cpu=0
[ "$d" -gt 0 ] && cpu=$((db*100/d))
echo "cpu=$cpu"
echo "cpus=$(nproc 2>/dev/null || echo 1)"
read -r la _ < /proc/loadavg 2>/dev/null; echo "load1=$la"
awk '/^MemTotal:/{t=$2}/^MemAvailable:/{a=$2}END{printf "memtotal=%d\nmemused=%d\n",t*1024,(t-a)*1024}' /proc/meminfo 2>/dev/null
df -kP "$HOME" 2>/dev/null | awk 'NR==2{printf "disktotal=%d\ndiskused=%d\n",$2*1024,$3*1024}'
echo "procs=$(ps -e --no-headers 2>/dev/null | wc -l | tr -d ' ')"
awk '{printf "uptime=%d\n",$1}' /proc/uptime 2>/dev/null
echo "host=$(hostname 2>/dev/null)"
`

func (s *Server) projectStats(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	st, err := s.orch.Status(r.Context(), u.Username, p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if st.State != "running" {
		writeJSON(w, http.StatusOK, projectStatsView{State: st.State})
		return
	}
	guest := s.guestUserFor(u.Username)
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	view := projectStatsView{State: "running"}
	if code, out, err := s.orch.Exec(ctx, st.ContainerID, orchestrator.ExecSpec{
		User: guest,
		Env:  []string{"HOME=/home/" + guest},
		Cmd:  []string{"bash", "-c", statsScript},
	}); err == nil && code == 0 {
		parseStats(out, &view)
	}
	// IP is resolved host-side (authoritative) rather than from the guest.
	if ip, err := s.orch.IPAddress(ctx, u.Username, p.ID); err == nil {
		view.IP = ip
	}
	writeJSON(w, http.StatusOK, view)
}

func parseStats(out string, v *projectStatsView) {
	atoi := func(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
	atoi64 := func(s string) int64 { n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64); return n }
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "cpu":
			v.CPUPercent = atoi(val)
		case "cpus":
			v.CPUCount = atoi(val)
		case "load1":
			v.Load1, _ = strconv.ParseFloat(strings.TrimSpace(val), 64)
		case "memtotal":
			v.MemTotal = atoi64(val)
		case "memused":
			v.MemUsed = atoi64(val)
		case "disktotal":
			v.DiskTotal = atoi64(val)
		case "diskused":
			v.DiskUsed = atoi64(val)
		case "procs":
			v.Processes = atoi(val)
		case "uptime":
			v.Uptime = atoi64(val)
		case "host":
			v.Host = strings.TrimSpace(val)
		}
	}
}
