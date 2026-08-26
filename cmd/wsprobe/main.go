// Command wsprobe is a development/integration test client for styrman's
// terminal WebSocket. It attaches to a conversation, optionally sends input,
// and prints what the terminal produced — letting shell scripts verify the
// full browser path without a browser.
//
//	wsprobe -url ws://127.0.0.1:8080/ws/term/alpha/1?kind=shell \
//	        -send "echo hello-$$\r" -wait 3s
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/coder/websocket"
)

func main() {
	url := flag.String("url", "", "terminal WebSocket URL")
	send := flag.String("send", "", "input to send after connecting (\\r for Enter)")
	wait := flag.Duration("wait", 3*time.Second, "how long to collect output")
	flag.Parse()
	if *url == "" {
		fmt.Fprintln(os.Stderr, "wsprobe: -url required")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *wait+30*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, *url, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wsprobe: dial:", err)
		os.Exit(1)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(-1)

	if *send != "" {
		// Give tmux a beat to attach before typing.
		time.Sleep(500 * time.Millisecond)
		if err := conn.Write(ctx, websocket.MessageBinary, []byte(unescape(*send))); err != nil {
			fmt.Fprintln(os.Stderr, "wsprobe: write:", err)
			os.Exit(1)
		}
	}

	deadline := time.Now().Add(*wait)
	for time.Now().Before(deadline) {
		readCtx, readCancel := context.WithDeadline(ctx, deadline)
		_, data, err := conn.Read(readCtx)
		readCancel()
		if err != nil {
			break
		}
		os.Stdout.Write(data)
	}
	fmt.Println()
	conn.Close(websocket.StatusNormalClosure, "probe done")
}

func unescape(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'r':
				out = append(out, '\r')
				i++
				continue
			case 'n':
				out = append(out, '\n')
				i++
				continue
			}
		}
		out = append(out, s[i])
	}
	return string(out)
}
