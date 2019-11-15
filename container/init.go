package container

import (
	"syscall"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"io/ioutil"
	"strings"
	"fmt"
)


// RunCotainerInitProcess 创建真正的容器进程
func RunCotainerInitProcess() error {

	/*
		MS_ONEXC 本文件系统允许允许其他程序
		MS_NOSUDI 本文件系统运行时，不允许 set_uid 和 set_gid
		MS_NODEV linux 2.4 之后有的 mount 默认参数
	*/
	
	cmdList := readUserCmd()

	if cmdList == nil || len(cmdList) == 0 {
		return fmt.Errorf("Run container get user command error, command is nil")
	}

	log.Debugf("Run container get user command : %v", cmdList)

	defaultMountFlages := syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV

	syscall.Mount("proc", "/proc", "proc", uintptr(defaultMountFlages), "")
	// mount -t proc proc /proc

	// 调用 exec.LookPath 在系统的 PATH 中寻找命令的绝对路径
	absPath, err := exec.LookPath(cmdList[0])

	if err != nil {
		log.Errorf("Exec Loop Path error : %v", err)
	}

	log.Debugf("Find command absPATH : %s", absPath)

	// exec 创建真正的容器进程
	 if err := syscall.Exec(absPath, cmdList[0:], os.Environ()); err != nil {
		log.Errorf(err.Error())
	}

	return nil

}

// readUserCmd 获取用户参数
func readUserCmd() []string {

	// readPipe是下标为 3 的文件描述符
	readPipe := os.NewFile(uintptr(3), "pipe")

	cmdByte, err := ioutil.ReadAll(readPipe)

	if err != nil {
		log.Errorf("get user's cmd error : %v", err)

		return nil
	}

	// 传过来的是字节
	cmdString := string(cmdByte)

	cmdList := strings.Split(cmdString, " ")

	return cmdList
}