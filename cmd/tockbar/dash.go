package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// dashIdle is how long the dashboard runs with no request before it stops.
const dashIdle = 30 * time.Minute

// dashWait is how long the dashboard may take to print its link.
const dashWait = 10 * time.Second

// linkPrefix starts the line tock web prints its link on.
const linkPrefix = "tock web: "

// dash runs the web dashboard as a `tock web` child: the menu bar app itself
// never links an HTTP stack. The child stops itself after dashIdle with no
// request; the next click starts a fresh one.
type dash struct {
	cmd func() *exec.Cmd // the tock web command to run

	mu   sync.Mutex // guards proc and link, which the wait goroutine clears
	proc *exec.Cmd
	link string // "http://127.0.0.1:<port>/#<token>"
}

// URL returns the link to view ("" = today), starting the dashboard when
// none runs.
func (d *dash) URL(view string) (string, error) {
	d.mu.Lock()
	link := d.link
	d.mu.Unlock()
	if link == "" {
		var err error
		if link, err = d.start(); err != nil {
			return "", err
		}
	}
	if view == "" {
		return link, nil
	}
	base, frag, _ := strings.Cut(link, "#")
	return base + "?v=" + view + "#" + frag, nil
}

func (d *dash) start() (string, error) {
	c := d.cmd()
	out, err := c.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("dash: pipe: %w", err)
	}
	if err := c.Start(); err != nil {
		return "", fmt.Errorf("dash: start: %w", err)
	}
	got := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(out)
		link := ""
		if sc.Scan() {
			link, _ = strings.CutPrefix(sc.Text(), linkPrefix)
		}
		got <- link
		io.Copy(io.Discard, out) // keep the pipe open so the child never blocks on a write
	}()
	var link string
	select {
	case link = <-got:
	case <-time.After(dashWait):
	}
	if !strings.HasPrefix(link, "http://127.0.0.1:") || !strings.Contains(link, "#") {
		c.Process.Kill()
		c.Wait()
		return "", errors.New("dash: tock web printed no link")
	}
	d.mu.Lock()
	d.proc, d.link = c, link
	d.mu.Unlock()
	go func() {
		c.Wait()
		d.mu.Lock()
		if d.proc == c {
			d.proc, d.link = nil, ""
		}
		d.mu.Unlock()
	}()
	return link, nil
}

// Close stops the dashboard.
func (d *dash) Close() {
	d.mu.Lock()
	c := d.proc
	d.proc, d.link = nil, ""
	d.mu.Unlock()
	if c != nil {
		c.Process.Kill()
	}
}
