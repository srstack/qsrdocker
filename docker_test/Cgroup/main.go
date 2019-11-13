package main

import (
	"fmt"
	"os/exec"
	"path"
	"os"
	"io/ioutil"
	"syscall"
	"strconv"
	"log"
)

// 设置系统hierachy路径常量

const cgroupHierarchyMemoryPath = "/sys/fs/cgroup/memory"


func main() {
	// fork出来的进程的初始化命令
	cmd := exec.Command("sh")

	// 获取当前用户

	uid := syscall.Getuid()  // 字符串转int
	gid := syscall.Getgid()

	log.Printf("uid=%d,gid=%d", uid, gid)

	// Namespace 的系统调用参数
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | 
			syscall.CLONE_NEWIPC |  // IPC 调用参数
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
	
    // cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)} // 以 qsr 用户执行 os.exec

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	err := cmd.Start()

	if err != nil {
		// 等价于fmt.Println(); os.Exit(1);
		log.Fatal(err)
	} else {
		err := os.Mkdir(path.Join(cgroupHierarchyMemoryPath, "testmemorylimit"), 0755) // 创建cgroup
		
		if err != nil {
			fmt.Println(err) 
	   }

	    // ioutil.WriteFile 属于 w 模式 
		ioutil.WriteFile(
			path.Join(cgroupHierarchyMemoryPath, "testmemorylimit", "tasks"), // 将限制进程pid写入cgroup tasks中
			[]byte(strconv.Itoa(cmd.Process.Pid)), // 字节
			0644,
		)

		ioutil.WriteFile(
			path.Join(cgroupHierarchyMemoryPath, "testmemorylimit", "memory.limit_in_bytes"),
			[]byte("50m"), // 限制内存使用50M
			0644,
		)
	}
	cmd.Process.Wait()
}