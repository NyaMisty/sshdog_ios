package main

import (
	"fmt"
	exec2 "github.com/Matir/sshdog/exec"
	"io"
	"os"
	"os/exec"
	"time"
)

//type Cmd exec.Cmd
////go:linkname InitStdin os/exec.(*Cmd).stdin
//func InitStdin(p *Cmd) (f *os.File, err error)
//
//func test() {
//	cmd := exec.Command("cmd.exe")
//	pip, _ := cmd.StdinPipe()
//	defer pip.Close()
//	InitStdin((*Cmd)(unsafe.Pointer(cmd)))
//}

func main() {
	cmd := exec.Command("/bin/sh", "/tmp/111.sh")
	outPipe, _ := cmd.StdoutPipe()

	err := exec2.Start(cmd)
	fmt.Printf("posixspawn! pid:%d err:%v\n", cmd.Process.Pid, err)

	io.Copy(os.Stdout, outPipe)
	time.Sleep(time.Second * 5)
	fmt.Printf("finished")

	fmt.Printf("111\n")
}
