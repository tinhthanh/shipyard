package terminal

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// readAll reads what the console shows until it ends, or fails after a while.
func readAll(t *testing.T, p PTY) string {
	t.Helper()
	got := make(chan []byte, 1)
	go func() {
		var b bytes.Buffer
		io.Copy(&b, p)
		got <- b.Bytes()
	}()
	select {
	case b := <-got:
		return string(b)
	case <-time.After(20 * time.Second):
		t.Fatal("the console's output never ended")
		return ""
	}
}

func TestAProgramInAPseudoConsoleShowsItsOutputAndExits(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "echo hello from conpty")
	p, err := Start(cmd, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if out := readAll(t, p); !strings.Contains(out, "hello from conpty") {
		t.Fatalf("output %q", out)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestAPseudoConsoleReportsTheExitCode(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit 3")
	p, err := Start(cmd, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	readAll(t, p)
	var ee *exec.ExitError
	if err := cmd.Wait(); !errors.As(err, &ee) || ee.ExitCode() != 3 {
		t.Fatalf("wait: %v, want exit status 3", err)
	}
}

func TestWhatIsTypedReachesTheProgram(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/q", "/k")
	p, err := Start(cmd, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := Resize(p, 120, 30); err != nil {
		t.Fatalf("resize: %v", err)
	}
	if _, err := p.Write([]byte("echo typed %COMPUTERNAME:~0,0%123\r\nexit\r\n")); err != nil {
		t.Fatal(err)
	}
	if out := readAll(t, p); !strings.Contains(out, "typed 123") {
		t.Fatalf("output %q", out)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestClosingThePseudoConsoleEndsItsProgram(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/q", "/k")
	p, err := Start(cmd, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	p.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the program outlived its console")
	}
}

func TestAPseudoConsoleClosesTwiceWhileItIsRead(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/q", "/k")
	p, err := Start(cmd, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	read := make(chan struct{})
	go func() {
		io.Copy(io.Discard, p)
		close(read)
	}()
	time.Sleep(200 * time.Millisecond)
	p.Close()
	p.Close()
	select {
	case <-read:
	case <-time.After(20 * time.Second):
		t.Fatal("a read went on after the console closed")
	}
	cmd.Wait()
}
