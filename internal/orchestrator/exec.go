package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// ExecSpec describes a command to run inside a project container.
type ExecSpec struct {
	Cmd     []string
	User    string // "" = container default (root)
	Env     []string
	WorkDir string
}

// maxExecOutput bounds how much of a non-interactive guest command's output we
// hold in memory. The guest is untrusted and root inside its own VM, so it
// controls the size of everything styrman execs there; without a cap one
// project could OOM the whole shared server (H4/H7). 2 MiB comfortably covers
// readiness probes, tmux bookkeeping, git status/diff, and the stats script.
const maxExecOutput = 2 << 20

// cappedBuffer accumulates up to max bytes, then keeps "accepting" writes but
// discards them — so a hostile guest's output is bounded in memory yet still
// fully drained, letting the command complete and its exit code be read.
type cappedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		if room >= len(p) {
			c.buf.Write(p)
		} else {
			c.buf.Write(p[:room])
			c.truncated = true
		}
	} else {
		c.truncated = true
	}
	return len(p), nil // always accept so StdCopy keeps draining the guest
}

func (c *cappedBuffer) String() string {
	if c.truncated {
		return c.buf.String() + "\n… (truncated)"
	}
	return c.buf.String()
}

// Exec runs a non-interactive command and returns its exit code and combined
// output. Used for readiness probes and tmux session management.
func (o *Orchestrator) Exec(ctx context.Context, containerID string, spec ExecSpec) (int, string, error) {
	create, err := o.docker.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          spec.Cmd,
		User:         spec.User,
		Env:          spec.Env,
		WorkingDir:   spec.WorkDir,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return -1, "", fmt.Errorf("exec create: %w", err)
	}
	att, err := o.docker.ContainerExecAttach(ctx, create.ID, container.ExecStartOptions{})
	if err != nil {
		return -1, "", fmt.Errorf("exec attach: %w", err)
	}
	defer att.Close()

	buf := &cappedBuffer{max: maxExecOutput}
	// Non-TTY exec streams are multiplexed; demux stdout+stderr together.
	// The Docker hijack has no ctx watchdog, so a wedged guest command would
	// block StdCopy (and leak this goroutine + the hijacked conn) forever.
	// Run it in a goroutine and abandon on ctx cancel — att.Close() (deferred)
	// then unblocks the reader.
	copyDone := make(chan struct{})
	go func() {
		_, _ = stdcopy.StdCopy(buf, buf, att.Reader)
		close(copyDone)
	}()
	select {
	case <-copyDone:
	case <-ctx.Done():
		att.Close() // unblock the StdCopy read
		<-copyDone  // wait for the goroutine to stop writing buf (no race)
		return -1, buf.String(), ctx.Err()
	}

	// Exit code is only final once the process is gone; poll briefly.
	for i := 0; i < 20; i++ {
		insp, err := o.docker.ContainerExecInspect(ctx, create.ID)
		if err != nil {
			return -1, buf.String(), err
		}
		if !insp.Running {
			return insp.ExitCode, buf.String(), nil
		}
		select {
		case <-ctx.Done():
			return -1, buf.String(), ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return -1, buf.String(), fmt.Errorf("exec did not finish")
}

// InteractiveExec is a live TTY exec stream into a container.
type InteractiveExec struct {
	ID      string
	o       *Orchestrator
	conn    io.ReadWriter
	closeFn func()
}

// StartInteractive creates and attaches a TTY exec (ADR-002: this is the
// terminal transport — the raw stream carries PTY bytes both ways).
func (o *Orchestrator) StartInteractive(ctx context.Context, containerID string, spec ExecSpec, cols, rows uint) (*InteractiveExec, error) {
	create, err := o.docker.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          spec.Cmd,
		User:         spec.User,
		Env:          spec.Env,
		WorkingDir:   spec.WorkDir,
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		ConsoleSize:  &[2]uint{rows, cols},
	})
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}
	att, err := o.docker.ContainerExecAttach(ctx, create.ID, container.ExecStartOptions{
		Tty:         true,
		ConsoleSize: &[2]uint{rows, cols},
	})
	if err != nil {
		return nil, fmt.Errorf("exec attach: %w", err)
	}
	return &InteractiveExec{
		ID:      create.ID,
		o:       o,
		conn:    att.Conn,
		closeFn: att.Close,
	}, nil
}

// Read reads raw PTY output.
func (e *InteractiveExec) Read(p []byte) (int, error) { return e.conn.Read(p) }

// Write writes raw input to the PTY.
func (e *InteractiveExec) Write(p []byte) (int, error) { return e.conn.Write(p) }

// Resize changes the exec's TTY size.
func (e *InteractiveExec) Resize(ctx context.Context, cols, rows uint) error {
	return e.o.docker.ContainerExecResize(ctx, e.ID, container.ResizeOptions{Width: cols, Height: rows})
}

// ExitCodeWait polls the exec for up to wait: dockerd's Running flag lags the
// stream EOF by a few milliseconds, so a single inspect would misclassify a
// genuine exit as a detach. Returns (-1, false) if still running after wait.
func (e *InteractiveExec) ExitCodeWait(ctx context.Context, wait time.Duration) (int, bool) {
	deadline := time.Now().Add(wait)
	for {
		insp, err := e.o.docker.ContainerExecInspect(ctx, e.ID)
		if err == nil && !insp.Running {
			return insp.ExitCode, true
		}
		if time.Now().After(deadline) {
			return -1, false
		}
		select {
		case <-ctx.Done():
			return -1, false
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Close tears down the attached stream.
func (e *InteractiveExec) Close() { e.closeFn() }
