//go:build darwin || ios

// Copyright 2016 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// "Daemonize" a process on linux
package daemon

import (
	"fmt"
	"github.com/Matir/sshdog/dbglog"
	exec2 "github.com/Matir/sshdog/exec"
	"os"
	"os/exec"
	"syscall"
)

var dbg = dbglog.Dbg

// Attempts to restart this process in the background.
// This is not a *true* daemonize, as the process is
// restarted.
func Daemonize(f DaemonWorker) error {
	var err error
	executable, _ := os.Executable()
	proc := exec.Command(executable, "daemon")
	proc.SysProcAttr = &syscall.SysProcAttr{}
	proc.SysProcAttr.Setpgid = true
	proc.SysProcAttr.Pgid = 0
	output, err := os.OpenFile("sshdog.log", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		dbg.Fatalf("failed to open log file: %v", err)
		return err
	}
	dbg.Debug("opened daemon log file: %v", output.Fd())
	//if userInfo, err := user.Current(); err == nil {
	//	proc.Dir = userInfo.HomeDir
	//}
	proc.Stdout = output
	proc.Stderr = output
	//proc.Stdout = os.Stdout
	//proc.Stdout = os.Stderr
	err = exec2.Start(proc)
	if err != nil {
		dbg.Fatalf("failed to spawn daemon: %v", err)
		return err
	}
	return nil
}

func DaemonizeOld(f DaemonWorker) error {
	if done, err := alreadyDaemonized(); err != nil {
		return err
	} else if done {
		waitFunc, _ := f()
		waitFunc()
		return nil
	}
	fmt.Printf("Daemonizing...")

	bin, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(bin)
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// No I/O
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Setup some other stuff
	cmd.Dir = "/"

	// Prevent signals from getting there
	cmd.SysProcAttr.Setsid = true

	//err = cmd.Start()
	err = exec2.Start(cmd)
	if err == nil {
		os.Exit(0) // kill the parent
	}
	return err
}

func alreadyDaemonized() (bool, error) {
	if dir, err := os.Getwd(); err != nil {
		return false, err
	} else if dir == "/" {
		return true, nil
	}
	return false, nil
}
