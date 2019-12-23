package container

import (
	"os"
	"os/exec"
	"syscall"
	log "github.com/sirupsen/logrus"
	"strings"
)

var (
	RootDir					string = "/root/var/qsrdocker"
	ImageDir				string = "/root/var/qsrdocker/image"
	// MountDir imagedir  imageName : imageID 的映射, 将映射写在 image.json文件中
	MountDir				string = "/root/var/qsrdocker/mnt"
)

// NewParentProcess 创建 runC 的守护进程
func NewParentProcess(tty bool, containerID, imageName, volume string) (*exec.Cmd, *os.File) {

	/*
		1. 第一个参数为初始化 init RunCotainerInitProcess
		2. 通过系统调用 exec 运行 init 初始化 qsrdocker
			执行当前 filename 对应的程序，并且当前进程的镜像、数据和堆栈信息
			重新启动一个新的程序/覆盖当前进程
			确保容器内的一个进程(init)是由我们指定的进程，而不是容器初始化init进程
			容器内部调用
	*/

	// 打印 command 
	//log.Debugf("Create Parent Process cmd: %v", command)
	
	readPipe, writePipe, err := NewPipe()
	if err != nil {
		log.Errorf("Create New pipe err: %v", err)
		return nil, nil
	}

	// exec 方式直接运行 qsrdocker init 
	cmd := exec.Command("/proc/self/exe", "init") // 执行 initCmd

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

	// 传入管道问价读端fd
	cmd.ExtraFiles = []*os.File {readPipe}
	// 一个进程的文件描述符默认 0 1 2 代表 输入 输出 错误 
	// readPipe 为外带的第四个文件描述符 下标为 3

	// 设置进程环境变量
	//cmd.Env = append(os.Environ(), envSlice...)

	// 创建容器运行目录
	// 创建容器映射数据卷
	// 创建挂载卷 （数据持久化）
	err = NewWorkSpace(imageName, containerID, volume)

	if err != nil {
		log.Errorf("Can't create docker workspace error : %v", err)
		return nil, nil
	}

	// 设置进程运行目录
	cmd.Dir = strings.Join([]string{MountDir, containerID, "merged"}, "/")
	return cmd, writePipe // 返回给 Run 写端fd，用于接收用户参数
}

// NewPipe 创建匿名管道实现 runC进程和parent进程通信
func NewPipe() (*os.File, *os.File, error) {
	read, write, err := os.Pipe() //创建管道，半双工模型
	if err != nil {
		return nil, nil, err
	}
	return read, write, nil
}