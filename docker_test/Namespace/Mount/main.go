package main

import (
	"os/exec" // 执行shell命令
	"syscall" // 调用底层操作系统
	"os" // 标准输入输出错误
	"log"  // 日志库
	// 仅此介绍一遍
)

func main() {
	// fork出来的进程的初始化命令
	cmd := exec.Command("sh")

	// Namespace 的系统调用参数
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | 
			syscall.CLONE_NEWIPC |  // IPC 调用参数
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS, // 史上第一个 Namespace
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	err := cmd.Run()

	if err != nil {
		// 等价于fmt.Println(); os.Exit(1);
		log.Fatal(err)
	}

}