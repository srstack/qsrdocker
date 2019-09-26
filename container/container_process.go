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
	args := []string{"init", command}
	cmd := exec.Command("/proc/self/exe", args...)

	uid := syscall.Getuid() // 字符串转int
	gid := syscall.Getgid()

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
