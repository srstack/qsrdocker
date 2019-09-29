package container

import (
	"os"
	"os/exec"
	"syscall"

	log "github.com/sirupsen/logrus"
)

func RunCotainerInitProcess(command string, args []string) error {
	log.Infof("command : %s", command)

	/*
		MS_ONEXC 本文件系统允许允许其他程序
		MS_NOSUDI 本文件系统运行时，不允许 set_uid 和 set_gid
		MS_NODEV linux 2.4 之后有的 mount 默认参数
	*/
	defaultMountFlages := syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV

	syscall.Mount("proc", "/proc", "proc", uintptr(defaultMountFlages), "")
	// mount -t proc proc /proc

	argv := []string{command}

	err := syscall.Exec(command, argv, os.Environ())

	if err != nil {
		log.Errorf(err.Error())
	}

	return nil

}

func NewParentProcess(tty bool, command string) *exec.Cmd {

	/*
		1. 第一个参数为初始化 init RunCotainerInitProcess
		2. 通过系统调用 exec 运行 init 初始化 qsrdocker
			执行当前 filename 对应的程序，并且覆盖前台(bash/sh)/当前进程的镜像、数据和堆栈信息
			重新启动一个新的程序/覆盖当前进程
			确保容器内的一个进程(init)是由我们指定的进程

	*/

	args := []string{"init", command}
	cmd := exec.Command("/proc/self/exe", args...)

	uid := syscall.Getuid() // 字符串转int
	gid := syscall.Getgid()

	// 设置namespace
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC | // IPC 调用参数
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS | // 史上第一个 Namespace
			syscall.CLONE_NEWUSER |
			syscall.CLONE_NEWNET,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0, // 映射为root
				HostID:      uid,
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0, // 映射为root
				HostID:      gid,
				Size:        1,
			},
		},
	}

	if tty {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	return cmd
}
