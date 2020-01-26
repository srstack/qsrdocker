package main

import (
	"fmt"
	"encoding/json"
	"strings"
	"os"
	"time"
	"syscall"
	"os/exec"
	"path"
	"qsrdocker/container"

	log "github.com/sirupsen/logrus"
)

func inspectContainer(containerName string) {
	// 获取containerInfo信息
	containerInfo, err := container.GetContainerInfoByNameID(containerName)
	if err != nil {
		log.Errorf("Get containerInfo fail : %v", err)
		return
	}

	// 存入数据
	containerInfoBytes, err := json.MarshalIndent(containerInfo, " ", "    ")
	if err != nil {
		log.Errorf("Get container %v Info err : %v", containerName, err)
		return 
	}

	containerInfoStr := strings.Join([]string{string(containerInfoBytes), "\n"}, "")

	fmt.Fprint(os.Stdout, containerInfoStr)
	
}

// stopContainer 停止容器
func stopContainer(containerName string, sleepTime int) {
	containerID, err := container.GetContainerIDByName(containerName)
	
	if strings.Replace(containerID, " ", "", -1) == "" || err != nil {
		log.Errorf("Get containerID fail : %v", err)
		return
	}

	log.Debugf("Get containerID success  id : %v", containerID)

	// 获取containerInfo信息
	containerInfo, err := container.GetContainerInfoByNameID(containerID)
	if err != nil {
		log.Errorf("Get containerInfo fail : %v", err)
		return
	}

	if !containerInfo.Status.Running{
		log.Errorf("Stop container fail, container is not running : %v", err)
		return
	}

	pid := containerInfo.Status.Pid

	if sleepTime > 0{
		// 睡眠 sleepTime 秒后
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}

	// 调用系统调用发送信号 SIGTERM
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		log.Errorf("Stop container %v error %v", containerName, err)
		return
	}

	log.Debugf("Stop container %v success", containerName)
	
	// 设置当前
	containerInfo.Status.StatusSet("Paused")
	containerInfo.Status.Pid = -1
	
	// 持久化 container Info 
	container.RecordContainerInfo(containerInfo, containerID)
	
}


// removeContainer 删除容器
func removeContainer(containerName string, Force, volume bool) {
	containerID, err := container.GetContainerIDByName(containerName)
	
	if strings.Replace(containerID, " ", "", -1) == "" || err != nil {
		log.Errorf("Get containerID fail : %v", err)
		return
	}

	log.Debugf("Get containerID success  id : %v", containerID)

	// 获取containerInfo信息
	containerInfo, err := container.GetContainerInfoByNameID(containerID)
	if err != nil {
		log.Errorf("Get containerInfo fail : %v", err)
		return
	}

	// 容器running状态且未设置 Force
	if containerInfo.Status.Running {
		if Force {
			// 强制退出
			stopContainer(containerName, 0)
			
		} else {
			log.Errorf("Remove container %v fail, container is running", containerName)
			return
		}
	}

	RemoveContainerNameInfo(containerID)

	// 删除工作目录
	//if err := container.DeleteWorkSpace(containerID, volumes); err != nil {
	if err := container.DeleteWorkSpace(containerID); err != nil {
		log.Errorf("Error: %v", err)
	}
	
	// 删除 cgroup
	// 存在问题，由于目标进程和qsrdocker remove 进程没有父子关系
	// 所以无法删除 /sys/fs/cgroup/[subsystem]/qsrdocker/[containerID]
	containerInfo.Cgroup.Destroy()
	
	
	// 需要删除数据卷
	if volume {
		volumeMountInfoSlice := containerInfo.Mount

		for _, mountInfo := range volumeMountInfoSlice {
			if err := os.RemoveAll(mountInfo.Source); err != nil {
				log.Errorf("Remove Mount Bind Volume %s Error: %v", mountInfo.Source, err)
			} else {
				log.Debug("Remove Mount Bind Volume %s success", mountInfo.Source)
			}
		}
	}
}

// startContainer 启动容器
func startContainer(containerName string) {
	containerID, err := container.GetContainerIDByName(containerName)
	
	if strings.Replace(containerID, " ", "", -1) == "" || err != nil {
		log.Errorf("Get containerID fail : %v", err)
		return
	}

	log.Debugf("Get containerID success  id : %v", containerID)

	// 获取containerInfo信息
	containerInfo, err := container.GetContainerInfoByNameID(containerID)
	if err != nil {
		log.Errorf("Get containerInfo fail : %v", err)
		return
	}

	// 判断容器状态
	if containerInfo.Status.Running {
		log.Errorf(" This container %v is running, can't not start", containerName)
		return
	}

	// 检测挂载点是否存在异常
	health, err := container.MountPointCheckFuncMap[containerInfo.GraphDriver.Driver](containerInfo.GraphDriver.Data) 
	
	if !health || err != nil {
		log.Errorf(" Can't start container %v , workSpace is unhealthy", containerName)
		return
	}

	// 获取管道通信
	containerProcess, writeCmdPipe := StartParentProcess(containerInfo)

	if containerProcess == nil || writeCmdPipe == nil  {
		log.Errorf("New parent process error")
		return
	}

	log.Debugf("Get Qsrdocker : %v parent process and pipe success", containerID)

	if err := containerProcess.Start(); err != nil { // 启动真正的容器进程
		log.Error(err)
	}

	log.Debugf("Create container process success, pis is %v ", containerProcess.Process.Pid)
	
	// 设置进程状态 
	containerInfo.Status.StatusSet("Running")
	containerInfo.Status.Pid = containerProcess.Process.Pid
	containerInfo.Status.StartTime = time.Now().Format("2006-01-02 15:04:05")

	// 设置 cgroup
	// init 和 set 操作英格
	containerInfo.Cgroup.Init()
	containerInfo.Cgroup.Set()
	containerInfo.Cgroup.Apply(containerInfo.Status.Pid)

	log.Debugf("Create cgroup config: %+v", containerInfo.Cgroup.Resource)

	// 将用户命令发送给 init container 进程
	sendInitCommand(append([]string{containerInfo.Path,}, containerInfo.Args...), writeCmdPipe)
	
	// 将 containerInfo 存入 
	container.RecordContainerInfo(containerInfo, containerID)

	fmt.Printf("%v\n", containerID)
	
}


// StartParentProcess 创建 container 的启动进程
// 除了不设置 workspace 之外 其他的步骤和 container.NerParentProcess 基本一样
// 只能写重复代码了
func StartParentProcess(containerInfo *container.ContainerInfo) (*exec.Cmd, *os.File) {
	
	readCmdPipe, writeCmdPipe, err := container.NewPipe()
	
	if err != nil {
		log.Errorf("Create New Cmd pipe err: %v", err)
		return nil, nil
	}

	// exec 方式直接运行 qsrdocker init 
	cmd := exec.Command("/proc/self/exe", "init") // 执行 initCmd
	uid := syscall.Getuid() // 字符串转int
	gid := syscall.Getgid()

	log.Debugf("Get qsrdocker : %v uid : %v ; gid : %v", containerInfo.ID, uid, gid)

	if err = container.InitUserNamespace(); err != nil {
		log.Fatalf("UserNamespace err : %v", err)
	}

	// 设置 namespace
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC | // IPC 调用参数
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS | // 史上第一个 Namespace
			syscall.CLONE_NEWUSER |
			syscall.CLONE_NEWNET,
		UidMappings: []syscall.SysProcIDMap {
			{
				ContainerID: 0, // 映射为root
				HostID:      uid,
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap {
			{
				ContainerID: 0, // 映射为root
				HostID:      gid,
				Size:        1,
			},
		},
		GidMappingsEnableSetgroups: false,
	}

	log.Debugf("Set NameSpace to qsrdocker : %v", containerInfo.ID)

	

	// 容器信息目录 /[containerDir]/[containerID]/ 目录
	containerDir := path.Join(container.ContainerDir, containerInfo.ID)

	// 打开 log 文件
	containerLogFile := path.Join(containerDir, container.ContainerLogFile)
	logFileFd, err := os.Open(containerLogFile)
	if err != nil {
		log.Errorf("Get log file %s error %v", containerLogFile , err)
		return nil, nil
	}
	
	// 将标准输出 错误 重定向到 log 文件中
	cmd.Stdout = logFileFd
	cmd.Stderr = logFileFd

	// 传入管道问价读端fld
	cmd.ExtraFiles = []*os.File {readCmdPipe}
	// 一个进程的文件描d述符默认 0 1 2 代表 输入 输出 错误 
	// readCmdPipe 为外带的第四个文件描述符 下标为 3

	// 设置进程环境变量
	cmd.Env = append([]string{}, containerInfo.Env...)
	log.Debugf("Set container Env : %v", cmd.Env)
	
	// 设置进程运行目录
	cmd.Dir = container.GetMountPathFuncMap[containerInfo.GraphDriver.Driver](containerInfo.GraphDriver.Data)

	return cmd, writeCmdPipe
}



