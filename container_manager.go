package main

import (
	"fmt"
	"encoding/json"
	"strings"
	"os"
	"time"
	"syscall"
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

	// 检测容器状态
	containerInfo.Status.StatusCheck()

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

	containerInfo.Status.StatusCheck()

	// 容器running状态且未设置 Force
	if containerInfo.Status.Running {
		if Force {
			// 强制退出
			stopContainer(containerName, 0)
			
		} else {
			log.Errorf("Remove container %v , container is running", containerName)
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
				log.Errorf("Remove Mount Bind Volume %v Error: %v", mountInfo.Source, err)
			} else {
				log.Debug("Remove Mount Bind Volume %v success", mountInfo.Source)
			}
		}
	}
}