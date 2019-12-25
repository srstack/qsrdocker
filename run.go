package main

import (
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
	"github.com/srstack/qsrdocker/container"
	"github.com/srstack/qsrdocker/cgroups/subsystems"
	"github.com/srstack/qsrdocker/cgroups"
	"math/rand"
	"time"
)

// QsrdockerRun Docker守护进程启动
func QsrdockerRun(tty bool, cmdList, volumes []string, resCongfig *subsystems.ResourceConfig, 
	imageName, containerName string) {

	// 获取容器id
	containerID := randStringBytes(10)
	if containerName == "" {
		containerName = containerID
	}
	log.Debugf("Container name is %v", containerName)
	log.Debugf("Container ID is %v", containerID)
	
	// 获取管道通信
	parent, writePipe := container.NewParentProcess(tty, containerID, imageName)

	if parent == nil || writePipe == nil {
		log.Errorf("New parent process error")
		return
	}

	log.Debugf("Get Qsrdocker : %v parent process and pipe success", containerID)

	if err := parent.Start(); err != nil { // 启动真正的容器进程
		log.Error(err)
	}

	// 创建 mount bind 数据卷 挂载
	container.InitVolume(containerID, volumes)
	log.Debugf("InitVolume qsrdocker %v success", containerID)

	// 创建 cgroup_manager
	cgroupManager := cgroups.NewCgroupManager(containerID)
	defer cgroupManager.Destroy()

	// 初始化 /sys/fs/cgroup/[subsystem]/qsrdocker
	cgroupManager.Init()

	// set 设置资源
	cgroupManager.Set(resCongfig)

	// apply 应用资源(绑定PID至目标task)
	cgroupManager.Apply(parent.Process.Pid)

	// 将用户命令发送给守护进程 Parent
	sendInitCommand(cmdList, writePipe)

	if tty {
		parent.Wait()
		// 进程退出 exit

		// 删除工作目录
		if err := container.DeleteWorkSpace(containerID, volumes); err != nil {
			log.Errorf("Error: %v", err)
		}
	} 

	// 后台启动不需要 exit 了
	//os.Exit(-1)
}

// sendInitCommand 将用户命令发送给守护进程 Parent
func sendInitCommand(cmdList []string, writePipe *os.File) {
	cmd := strings.Join(cmdList, " ") // 转为字符串
	log.Debugf("Command : %v", cmd)

	// 将 cmd 字符串通过管道传给 守护进程 parent
	writePipe.WriteString(cmd)
	writePipe.Close() // 关闭写端
}


// randStringBytes 随机获取容器id
func randStringBytes(n int) string {

	// 确定容器id 位数
	letterBytes := "1234567890"

	// 以当前时间为种子创建 rand
	rand.Seed(time.Now().UnixNano())

	// 创建容器id
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}

	return string(b)
}