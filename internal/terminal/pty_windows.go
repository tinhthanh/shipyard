package terminal

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

// ErrNoPTY: Windows before 10 1809 has no pseudo console.
var ErrNoPTY = errors.New("this version of Windows has no pseudo console (it needs Windows 10 1809 or later)")

var procCreatePseudoConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole")

// PTY is a pseudo console (ConPTY) and the program running in it. What the
// program writes is read from it, what is written to it is the program's
// input. Closing it closes the console, which ends the program.
type PTY = *ConPTY

// ConPTY is Windows' pseudo console: unlike a Unix pseudo-terminal it has
// two pipes, one each way, and its program must be started with it named
// in its start-up attributes, which os/exec can't do.
type ConPTY struct {
	hpc     windows.Handle
	in      *os.File // the program's input
	out     *os.File // what the program writes
	console sync.Once
	closed  sync.Once
	mu      sync.Mutex
	process windows.Handle // 0 once the program has exited
}

// Start runs cmd in a new pseudo console of cols×rows. cmd.Wait works as
// after cmd.Start; the console's output ends once the program has exited.
// cmd.Stdin, Stdout and Stderr are not used.
func Start(cmd *exec.Cmd, cols, rows int) (PTY, error) {
	if cmd.Err != nil {
		return nil, cmd.Err
	}
	if procCreatePseudoConsole.Find() != nil {
		return nil, ErrNoPTY
	}
	// One pipe each way: the console reads the program's input from the
	// first and writes what it shows to the second.
	inRead, inWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		inRead.Close()
		inWrite.Close()
		return nil, err
	}
	var hpc windows.Handle
	err = windows.CreatePseudoConsole(coord(cols, rows), windows.Handle(inRead.Fd()), windows.Handle(outWrite.Fd()), 0, &hpc)
	// The console has its own copies of these ends now.
	inRead.Close()
	outWrite.Close()
	if err != nil {
		inWrite.Close()
		outRead.Close()
		return nil, err
	}
	p := &ConPTY{hpc: hpc, in: inWrite, out: outRead}
	pi, err := p.spawn(cmd)
	if err != nil {
		p.Close()
		return nil, err
	}
	windows.CloseHandle(pi.Thread)
	p.process = pi.Process
	// The output pipe stays open while the console does, after the program
	// has exited: close the console then, so a reader sees the end.
	go func() {
		windows.WaitForSingleObject(pi.Process, windows.INFINITE)
		p.closeConsole()
		p.mu.Lock()
		windows.CloseHandle(p.process)
		p.process = 0
		p.mu.Unlock()
	}()
	if cmd.Process, err = os.FindProcess(int(pi.ProcessId)); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

func (p *ConPTY) spawn(cmd *exec.Cmd) (*windows.ProcessInformation, error) {
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	defer attrs.Delete()
	// The attribute's value is the console handle itself, not a pointer to it.
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&p.hpc)), unsafe.Sizeof(p.hpc)); err != nil {
		return nil, err
	}
	si := &windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(*si))
	// No standard handles of its own: without this flag a program started
	// by one whose output is a pipe writes to that pipe, not the console.
	si.Flags |= windows.STARTF_USESTDHANDLES
	app, err := windows.UTF16PtrFromString(cmd.Path)
	if err != nil {
		return nil, err
	}
	line, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(cmd.Args))
	if err != nil {
		return nil, err
	}
	var dir *uint16
	if cmd.Dir != "" {
		if dir, err = windows.UTF16PtrFromString(cmd.Dir); err != nil {
			return nil, err
		}
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	pi := new(windows.ProcessInformation)
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	if err := windows.CreateProcess(app, line, nil, nil, false, flags, envBlock(env), dir, &si.StartupInfo, pi); err != nil {
		return nil, err
	}
	return pi, nil
}

// envBlock is an environment as CreateProcess takes it: NAME=value strings,
// each ended by a NUL, then one more NUL.
func envBlock(env []string) *uint16 {
	var b []uint16
	for _, kv := range env {
		b = append(b, utf16.Encode([]rune(kv))...)
		b = append(b, 0)
	}
	if len(b) == 0 {
		b = append(b, 0)
	}
	b = append(b, 0)
	return &b[0]
}

func coord(cols, rows int) windows.Coord {
	return windows.Coord{X: int16(min(max(cols, 1), 10000)), Y: int16(min(max(rows, 1), 10000))}
}

// Read reads what the program shows; io.EOF once it has exited.
func (p *ConPTY) Read(b []byte) (int, error) {
	n, err := p.out.Read(b)
	if errors.Is(err, windows.ERROR_BROKEN_PIPE) {
		err = os.ErrClosed
	}
	return n, err
}

// Write types into the program.
func (p *ConPTY) Write(b []byte) (int, error) { return p.in.Write(b) }

// closeConsole closes the pseudo console, which ends the program if it is
// still running and ends the output.
func (p *ConPTY) closeConsole() {
	p.console.Do(func() { windows.ClosePseudoConsole(p.hpc) })
}

// Close ends the program and lets go of the console. It may be called more
// than once, and while a Read waits.
func (p *ConPTY) Close() error {
	p.closed.Do(func() {
		// Before Windows 11 24H2, closing the console waits for its output
		// to be read: close the reading end first so it can't wait on nobody.
		p.out.Close()
		p.closeConsole()
		p.in.Close()
		// Closing the console asks its programs to close; one that doesn't
		// is ended.
		p.mu.Lock()
		if p.process != 0 {
			windows.TerminateProcess(p.process, 1)
		}
		p.mu.Unlock()
	})
	return nil
}

// Resize tells the program its console's size changed.
func Resize(p PTY, cols, rows int) error {
	if cols <= 0 || rows <= 0 || cols > 10000 || rows > 10000 {
		return nil
	}
	return windows.ResizePseudoConsole(p.hpc, coord(cols, rows))
}

// Size is the console's size in cells.
func Size(fd uintptr) (cols, rows int, err error) { return term.GetSize(int(fd)) }

// MakeRaw puts the console in raw mode and returns how to restore it.
func MakeRaw(fd uintptr) (restore func(), err error) {
	st, err := term.MakeRaw(int(fd))
	if err != nil {
		return nil, err
	}
	return func() { term.Restore(int(fd), st) }, nil
}

// IsTerminal reports whether fd is a console.
func IsTerminal(fd uintptr) bool { return term.IsTerminal(int(fd)) }
