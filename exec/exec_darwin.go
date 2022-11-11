//go:build darwin || ios

package exec

// #include <stdlib.h>
// #include <spawn.h>
import "C"
import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

////go:linkname cmd_stdin os/exec.(*Cmd).stdin
//func cmd_stdin(p *exec.Cmd) (f *os.File, err error)
//
////go:linkname cmd_stdout os/exec.(*Cmd).stdout
//func cmd_stdout(p *exec.Cmd) (f *os.File, err error)
//
////go:linkname cmd_stderr os/exec.(*Cmd).stderr
//func cmd_stderr(p *exec.Cmd) (f *os.File, err error)

////go:linkname interfaceEqual os/exec.interfaceEqual
//func interfaceEqual(a, b any) bool

// go 1.18
type Cmd struct {
	// Path is the path of the command to run.
	//
	// This is the only field that must be set to a non-zero
	// value. If Path is relative, it is evaluated relative
	// to Dir.
	Path string

	// Args holds command line arguments, including the command as Args[0].
	// If the Args field is empty or nil, Run uses {Path}.
	//
	// In typical use, both Path and Args are set by calling Command.
	Args []string

	// Env specifies the environment of the process.
	// Each entry is of the form "key=value".
	// If Env is nil, the new process uses the current process's
	// environment.
	// If Env contains duplicate environment keys, only the last
	// value in the slice for each duplicate key is used.
	// As a special case on Windows, SYSTEMROOT is always added if
	// missing and not explicitly set to the empty string.
	Env []string

	// Dir specifies the working directory of the command.
	// If Dir is the empty string, Run runs the command in the
	// calling process's current directory.
	Dir string

	// Stdin specifies the process's standard input.
	//
	// If Stdin is nil, the process reads from the null device (os.DevNull).
	//
	// If Stdin is an *os.File, the process's standard input is connected
	// directly to that file.
	//
	// Otherwise, during the execution of the command a separate
	// goroutine reads from Stdin and delivers that data to the command
	// over a pipe. In this case, Wait does not complete until the goroutine
	// stops copying, either because it has reached the end of Stdin
	// (EOF or a read error) or because writing to the pipe returned an error.
	Stdin io.Reader

	// Stdout and Stderr specify the process's standard output and error.
	//
	// If either is nil, Run connects the corresponding file descriptor
	// to the null device (os.DevNull).
	//
	// If either is an *os.File, the corresponding output from the process
	// is connected directly to that file.
	//
	// Otherwise, during the execution of the command a separate goroutine
	// reads from the process over a pipe and delivers that data to the
	// corresponding Writer. In this case, Wait does not complete until the
	// goroutine reaches EOF or encounters an error.
	//
	// If Stdout and Stderr are the same writer, and have a type that can
	// be compared with ==, at most one goroutine at a time will call Write.
	Stdout io.Writer
	Stderr io.Writer

	// ExtraFiles specifies additional open files to be inherited by the
	// new process. It does not include standard input, standard output, or
	// standard error. If non-nil, entry i becomes file descriptor 3+i.
	//
	// ExtraFiles is not supported on Windows.
	ExtraFiles []*os.File

	// SysProcAttr holds optional, operating system-specific attributes.
	// Run passes it to os.StartProcess as the os.ProcAttr's Sys field.
	SysProcAttr *syscall.SysProcAttr

	// Process is the underlying process, once started.
	Process *os.Process

	// ProcessState contains information about an exited process,
	// available after a call to Wait or Run.
	ProcessState *os.ProcessState

	ctx             context.Context // nil means none
	lookPathErr     error           // LookPath error, if any.
	finished        bool            // when Wait was called
	childFiles      []*os.File
	closeAfterStart []io.Closer
	closeAfterWait  []io.Closer
	goroutine       []func() error
	errch           chan error // one send per goroutine
	waitDone        chan struct{}
}

//go:linkname closeDescriptors os/exec.(*Cmd).closeDescriptors
func closeDescriptors(cmd *Cmd, closers []io.Closer)
func (c *Cmd) closeDescriptors(closers []io.Closer) {
	closeDescriptors(c, closers)
}

//go:linkname stdin os/exec.(*Cmd).stdin
func stdin(cmd *Cmd) (f *os.File, err error)
func (c *Cmd) stdin() (f *os.File, err error) {
	return stdin(c)
}

//go:linkname stdout os/exec.(*Cmd).stdout
func stdout(cmd *Cmd) (f *os.File, err error)
func (c *Cmd) stdout() (f *os.File, err error) {
	return stdout(c)
}

//go:linkname stderr os/exec.(*Cmd).stderr
func stderr(cmd *Cmd) (f *os.File, err error)
func (c *Cmd) stderr() (f *os.File, err error) {
	return stderr(c)
}

//go:linkname envv os/exec.(*Cmd).envv
func envv(cmd *Cmd) (f *os.File, err error)
func (c *Cmd) envv() (f *os.File, err error) {
	return stderr(c)
}

func (c *Cmd) Start() error {
	if c.Path == "" && c.lookPathErr == nil {
		c.lookPathErr = errors.New("exec: no command")
	}
	if c.lookPathErr != nil {
		c.closeDescriptors(c.closeAfterStart)
		c.closeDescriptors(c.closeAfterWait)
		return c.lookPathErr
	}
	if c.Process != nil {
		return errors.New("exec: already started")
	}
	if c.ctx != nil {
		select {
		case <-c.ctx.Done():
			c.closeDescriptors(c.closeAfterStart)
			c.closeDescriptors(c.closeAfterWait)
			return c.ctx.Err()
		default:
		}
	}

	c.childFiles = make([]*os.File, 0, 3+len(c.ExtraFiles))
	type F func(*Cmd) (*os.File, error)
	for _, setupFd := range []F{(*Cmd).stdin, (*Cmd).stdout, (*Cmd).stderr} {
		fd, err := setupFd(c)
		if err != nil {
			c.closeDescriptors(c.closeAfterStart)
			c.closeDescriptors(c.closeAfterWait)
			return err
		}
		c.childFiles = append(c.childFiles, fd)
	}
	c.childFiles = append(c.childFiles, c.ExtraFiles...)

	if c.Env == nil {
		c.Env = syscall.Environ()
	}

	err := PosixSpawn(c)

	if err != nil {
		c.closeDescriptors(c.closeAfterStart)
		c.closeDescriptors(c.closeAfterWait)
		return err
	}

	c.closeDescriptors(c.closeAfterStart)

	// Don't allocate the channel unless there are goroutines to fire.
	if len(c.goroutine) > 0 {
		c.errch = make(chan error, len(c.goroutine))
		for _, fn := range c.goroutine {
			go func(fn func() error) {
				c.errch <- fn()
			}(fn)
		}
	}

	if c.ctx != nil {
		c.waitDone = make(chan struct{})
		go func() {
			select {
			case <-c.ctx.Done():
				c.Process.Kill()
			case <-c.waitDone:
			}
		}()
	}

	return nil
}

func PosixSpawn(cmd *Cmd) error {
	// int posix_spawn(pid_t *restrict pid, const char *restrict path,
	//       const posix_spawn_file_actions_t *file_actions,
	//       const posix_spawnattr_t *restrict attrp,
	//       char *const args[restrict], char *const envp[restrict]);
	//cmd := (*Cmd)(unsafe.Pointer(_cmd))
	path := cmd.Path
	args := cmd.Args
	envp := cmd.Env
	fmt.Printf("path: %s, args: %v, envp: %v", path, args, envp)

	var retval C.int = 0

	var sppath = C.CString(path)
	defer C.free(unsafe.Pointer(sppath))

	var spargv = make([]*C.char, len(args)+1)
	spargv[len(args)] = nil
	for i, argvEntry := range args {
		spargv[i] = C.CString(argvEntry)
	}
	defer func() {
		for _, c := range spargv {
			C.free(unsafe.Pointer(c))
		}
	}()

	var spenvp = make([]*C.char, len(envp)+1)
	spenvp[len(args)] = nil
	for i, envEntry := range envp {
		spenvp[i] = C.CString(envEntry)
	}
	defer func() {
		for _, c := range spenvp {
			C.free(unsafe.Pointer(c))
		}
	}()

	var spattr C.posix_spawnattr_t
	retval = C.posix_spawnattr_init(&spattr)
	if retval != 0 {
		return fmt.Errorf("posix_spawnattr_init returned %d", retval)
	}

	var spflags C.short = 0
	//spflags |= C.POSIX_SPAWN_SETEXEC

	retval = C.posix_spawnattr_setflags(&spattr, spflags)
	if retval != 0 {
		return fmt.Errorf("posix_spawnattr_setflags returned %d", retval)
	}

	var child_fd_actions C.posix_spawn_file_actions_t
	retval = C.posix_spawn_file_actions_init(&child_fd_actions)
	if retval != 0 {
		return fmt.Errorf("posix_spawn_file_actions_init returned %d", retval)
	}

	//// here we'll leak c.closeAfterStart, but who cares :)
	//stdinF, err := cmd_stdin(cmd)
	//if err != nil {
	//	return err
	//}
	//
	//stdoutF, err := cmd_stdout(cmd)
	//if err != nil {
	//	return err
	//}
	//
	//stderrF := stdoutF
	//if cmd.Stderr != nil && interfaceEqual(cmd.Stderr, cmd.Stdout) {
	//	// don't call cmd_stderr, or it will access c.childFiles which panics
	//} else {
	//	stderrF, err = cmd_stderr(cmd)
	//}
	//if err != nil {
	//	return err
	//}

	stdinF := cmd.childFiles[0]
	stdoutF := cmd.childFiles[1]
	stderrF := cmd.childFiles[2]

	fmt.Printf("%v, %v, %v\n", stdinF, stdoutF, stderrF)

	retval += C.posix_spawn_file_actions_adddup2(&child_fd_actions, C.int(stdinF.Fd()), 0)
	retval += C.posix_spawn_file_actions_adddup2(&child_fd_actions, C.int(stdoutF.Fd()), 1)
	retval += C.posix_spawn_file_actions_adddup2(&child_fd_actions, C.int(stderrF.Fd()), 2)

	if retval != 0 {
		return fmt.Errorf("posix_spawn_file_actions_add calls returned %d", retval)
	}

	oriCwd, err := os.Getwd()
	newCwdPtr, err := syscall.BytePtrFromString(cmd.Dir)
	if err != nil {
		return fmt.Errorf("BytePtrFromString returned %d", retval)
	}
	syscall.Syscall(syscall.SYS___PTHREAD_CHDIR, uintptr(unsafe.Pointer(newCwdPtr)), 0, 0)
	var pid C.pid_t = -1
	retval = C.posix_spawnp(
		&pid,                                   // pid
		sppath,                                 // file
		&child_fd_actions,                      // fileaction
		&spattr,                                // attr
		(**C.char)(unsafe.Pointer(&spargv[0])), // argv
		(**C.char)(unsafe.Pointer(&spenvp[0])), // envp
	)
	oriCwdPtr, err := syscall.BytePtrFromString(oriCwd)
	if err != nil {
		return fmt.Errorf("BytePtrFromString returned %d", retval)
	}
	syscall.Syscall(syscall.SYS___PTHREAD_CHDIR, uintptr(unsafe.Pointer(oriCwdPtr)), 0, 0)
	if retval != 0 {
		return fmt.Errorf("posix_spawnp returned %d", retval)
	}
	cmd.Process = &os.Process{
		Pid: int(pid),
	}
	return nil
}

func Start(cmd *exec.Cmd) error {
	_cmd := (*Cmd)(unsafe.Pointer(cmd))
	return _cmd.Start()
}

func Run(cmd *exec.Cmd) error {
	if err := Start(cmd); err != nil {
		return err
	}
	return cmd.Wait()
}
