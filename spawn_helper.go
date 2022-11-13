package main

import (
	exec2 "github.com/Matir/sshdog/exec"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

func handleSpawnHelper() {
	dbg.NewPrefix("[SpawnHelper]")

	ctty, err := strconv.Atoi(os.Args[2])
	if err != nil {
		dbg.Fatalf("invalid ctty: %v", ctty)
	}
	spawnArgs := os.Args[3:]

	dbg.Debug("checking tty fd...")
	tty := os.NewFile(uintptr(ctty), "tty")
	if err := tty.Sync(); err != nil {
		if !strings.Contains(err.Error(), "inappropriate ioctl for device") {
			dbg.Fatalf("tty fd is invalid! err: %v", err)
		}
	}

	dbg.Debug("setting up tty...")
	if sid, err := unix.Getsid(0); err != nil || sid != unix.Getpid() {
		if _, err := unix.Setsid(); err != nil {
			dbg.Debug("setsid fail: %v", err)
		}
	} else {
		dbg.Debug("already session leader, not doing setsid")
	}

	ioctlarg := 0
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, tty.Fd(), unix.TIOCSCTTY, uintptr(unsafe.Pointer(&ioctlarg))); errno != 0 {
		dbg.Debug("ioctl TIOCSCTTY fail: %v", err)
	}

	dbg.Debug("spawning original program")
	cmd := exec.Command(spawnArgs[0], spawnArgs[1:]...)
	cmd.Env = syscall.Environ()
	cmd.Dir, _ = os.Getwd()
	//f, _ := os.OpenFile("/tmp/sshdogspawn", os.O_RDWR|os.O_CREATE, 0777)
	//f.Write([]byte(spawnArgs[0]))
	//f.Close()

	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot: "exec",
	}
	err = exec2.Run(cmd)
	dbg.Fatalf("posix_spawnp unexpected exit: %v", err)
}
