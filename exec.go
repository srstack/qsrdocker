package main

import (
	"strconv"
	"path"
	"fmt"
	"os"
	"io/ioutil"
	"encoding/json"
	"strings"
	"os/exec"
	"qsrdocker/container"
	_ "qsrdocker/nsenter"
	// 导入并不使用
	// 当设置环境变量后,触发其 __attribute__ 函数

	log "github.com/sirupsen/logrus"
)

// ENVEXECPID 环境变量 Pid
const ENVEXECPID = "qsrdocker_pid"
// ENVEXECCMD 环境变量 cmd
const ENVEXECCMD = "qsrdocker_cmd"

// ExecContainer 登陆到已经创建好的 qsrdocker 
func ExecContainer(tty bool, containerName string, cmdList []string) {
	
	// 获得目标容器状态
	statusInfo, err := GetContainerStatusByName(containerName)
	if err != nil {
		log.Errorf("Exec container GetContainerStatusByName %s error %v", containerName, err)
		return
	}
	
	// 判断进程状态
	if !statusInfo.Running {
		log.Errorf("Exec container fail, Status is Dead and  pid %v is not exist", statusInfo.Pid)
		return
	}

	// 获取进程PID
	pid := strconv.Itoa(statusInfo.Pid)

	// 字符切片拼接
	cmdStr := strings.Join(cmdList, " ")
	log.Debugf("Container pid %s", pid)
	log.Debugf("Command %s", cmdStr)

	// 重新 fork/exec 执行自己
	// 触发__attribute__函数
	cmd := exec.Command("/proc/self/exe", "exec")

	// 标准输出输入错误
	if tty {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	
	// 设置环境变量
	os.Setenv(ENVEXECPID, pid)
	os.Setenv(ENVEXECCMD, cmdStr)
	
	// 获取进程环境变量  
	// 在 run 创建 container 时，可能设置了环境变量
	containerEnvSlice := getEnvSliceByPid(pid)

	// 将上面获取到的环境变量通过加入到 cmd 的环境变量中
	// 便于后面再一次调用时触发 nsenter
	// os.Environ() == os.Setenv() 的
	// containerEnvSlice == 容器创建时设置的
	cmd.Env = append(os.Environ(), containerEnvSlice...)

	if err := cmd.Run(); err != nil {
		log.Errorf("Exec container %s error %v", containerName, err)
	}
}


// GetContainerStatusByName 通过容器名获取容器进程 PID
func GetContainerStatusByName(containerName string) (*container.StatusInfo, error) {

	// 获取 container ID
	containerID, err := ContainerNameToID(containerName)

	if strings.Replace(containerID, " ", "", -1) == "" || err != nil {
		return nil, fmt.Errorf("Get containerID fail : %v", err)
	}
	
	containerConfigFile := path.Join(container.ContainerDir, containerID, container.ConfigName)

	// 获取容器配置信息
	configBytes, err := ioutil.ReadFile(containerConfigFile)
	if err != nil {
		return nil, err
	}
	
	// 反序列化
	var containerInfo container.ContainerInfo
	if err := json.Unmarshal(configBytes, &containerInfo); err != nil {
		return nil, err
	}

	// 检测当前状态
	containerInfo.Status.StatusCheck()

	// 持久化当前状态
	container.RecordContainerInfo(&containerInfo, containerID)
	
	return containerInfo.Status, nil
}

// getEnvSliceByPid 获取进程环境变量
func getEnvSliceByPid(pid string) []string {
	
	envPath :=  path.Join("/proc", pid, "environ")

	// 读取环境变量
	// 确定设置成功
	contentBytes, err := ioutil.ReadFile(envPath)
	if err != nil {
		log.Errorf("Read file %s error %v", envPath, err)
		return nil
	}
	
	// env split by \u0000
	// 默认格式
	envSlice := strings.Split(string(contentBytes), "\u0000")
	return envSlice
}
