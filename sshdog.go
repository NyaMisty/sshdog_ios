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

// TODO: High-level file comment.
package main

import (
	"fmt"
	"github.com/GeertJohan/go.rice"
	"github.com/Matir/sshdog/daemon"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type Debugger bool

func (d Debugger) Debug(format string, args ...interface{}) {
	if d {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "[DEBUG] %s\n", msg)
	}
}

var dbg Debugger = true

// Lookup the port number
func getPort(box *rice.Box) int16 {
	if len(os.Args) > 1 {
		if port, err := strconv.Atoi(os.Args[1]); err != nil {
			dbg.Debug("Error parsing %s as port: %v", os.Args[1], err)
		} else {
			return int16(port)
		}
	}
	if portData, err := box.String("port"); err == nil {
		portData = strings.TrimSpace(portData)
		if port, err := strconv.Atoi(portData); err != nil {
			dbg.Debug("Error parsing %s as port: %v", portData, err)
		} else {
			return int16(port)
		}
	}
	return 2222 // default
}

// Just check if a file exists
func fileExists(box *rice.Box, name string) bool {
	_, err := box.Bytes(name)
	return err == nil
}

// Should we daemonize?
func shouldDaemonize(box *rice.Box) bool {
	return fileExists(box, "daemon")
}

// Should we be silent?
func beQuiet(box *rice.Box) bool {
	return fileExists(box, "quiet")
}

var mainBox *rice.Box

func readExitInput() {
	lastStr := make([]byte, 4)
	endMark := []byte("exit")
	var startTime = time.Now()
	var firstErr error
	for {
		buf := make([]byte, 1)
		n, err := os.Stdin.Read(buf)
		if err != nil {
			if firstErr == nil && err == io.EOF {
				if time.Now().Sub(startTime) < 1*time.Second { // like ./sshdog < /dev/null
					dbg.Debug("We don't have stdin, not monitoring exit input!")
					return
				} // else the user may input an EOF after some time
			}
			if err != io.EOF {
				dbg.Debug("fatal: unknown read err: %v", err)
				os.Exit(1)
			} else {
				os.Exit(0)
			}
		}
		if n == 1 {
			lastStr = append(lastStr[1:], buf[0])
			//fmt.Printf("last input: %d\n", buf[0])
		}
		shouldExit := true
		for i := range lastStr {
			if lastStr[i] != endMark[i] {
				shouldExit = false
			}
		}
		if shouldExit {
			os.Exit(0)
		}
	}
}

func main() {
	mainBox = mustFindBox()

	if beQuiet(mainBox) {
		dbg = false
	}

	if shouldDaemonize(mainBox) {
		if err := daemon.Daemonize(daemonStart); err != nil {
			dbg.Debug("Error daemonizing: %v", err)
		}
	} else {
		go readExitInput()
		waitFunc, _ := daemonStart()
		if waitFunc != nil {
			waitFunc()
		}
	}
}

func mustFindBox() *rice.Box {
	// Overloading name 'rice' due to bug in rice to be fixed in 2.0:
	// https://github.com/GeertJohan/go.rice/issues/58
	rice := &rice.Config{
		LocateOrder: []rice.LocateMethod{
			rice.LocateAppended,
			rice.LocateEmbedded,
			rice.LocateWorkingDirectory,
		},
	}
	if box, err := rice.FindBox("config"); err != nil {
		panic(err)
	} else {
		return box
	}
}

// Actually run the implementation of the daemon
func daemonStart() (waitFunc func(), stopFunc func()) {
	server := NewServer()

	hasHostKeys := false
	for _, keyName := range keyNames {
		if keyData, err := mainBox.Bytes(keyName); err == nil {
			dbg.Debug("Adding hostkey file: %s", keyName)
			if err = server.AddHostkey(keyData); err != nil {
				dbg.Debug("Error adding public key: %v", err)
			}
			hasHostKeys = true
		}
	}
	if !hasHostKeys {
		if err := server.RandomHostkey(); err != nil {
			dbg.Debug("Error adding random hostkey: %v", err)
			return
		}
	}
	authSet := false
	if passwordData, err := mainBox.Bytes("password"); err == nil {
		dbg.Debug("Setting auth password.")
		server.SetAuthPassword(passwordData)
		authSet = true
	}
	if authData, err := mainBox.Bytes("authorized_keys"); err == nil {
		dbg.Debug("Adding authorized_keys.")
		server.AddAuthorizedKeys(authData)
		authSet = true
	}
	if !authSet {
		dbg.Debug("Neither password nor key was configured. We will not do any auth!")
		//return
	}
	server.ListenAndServe(getPort(mainBox))
	return server.Wait, server.Stop
}
