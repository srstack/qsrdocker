package main

import (
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
	"github.com/srstack/qsrdocker/container"
	"github.com/srstack/qsrdocker/cgroups/subsystems"
	"github.com/srstack/qsrdocker/cgroups"
)

// QsrdockerRun Docker守护进程启动
func QsrdockerRun(tty bool, cmdList []string, resCongfig *subsystems.ResourceConfig) {
	parent, writePipe := container.NewParentProcess(tty)

	if parent == nil {
		log.Errorf("New parent process error")
		return
	}

	if err := parent.Start(); err != nil { // 启动真正的容器进程
		log.Error(err)
	}

	// 创建 cgroup_manager
	cgroupManager := cgroups.NewCgroupManager("qsrdocker_cgroup")
	defer cgroupManager.Destroy()

	// set 设置资源
	cgroupManager.Set(resCongfig)

	// apply 应用资源(绑定PID至目标task)
	cgroupManager.Apply(parent.Process.Pid)

	// 将用户命令发送给守护进程 Parent
	sendInitCommand(cmdList, writePipe)

	if tty {
		parent.Wait()
	} 
	
	// 后台启动不需要 exit 了
	//os.Exit(-1)
}

// sendInitCommand 将用户命令发送给守护进程 Parent
func sendInitCommand(cmdList []string, writePipe *os.File) {
	cmd := strings.Join(cmdList, " ") // 转为字符串
	log.Debugf("command : %v", cmd)

	// 将 cmd 字符串通过管道传给 守护进程 parent
	writePipe.WriteString(cmd)
	writePipe.Close() // 关闭写端
}