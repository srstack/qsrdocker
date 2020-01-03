package main

import (
	"io/ioutil"
	"strings"
	"os"
	"text/tabwriter"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/srstack/qsrdocker/container"
)

// ListContainers 列出container信息
func listContainers(all bool) {
	// 获取 ContainerDir 下的文件
	containerDirs, err := ioutil.ReadDir(container.ContainerDir)
	if err != nil {
		log.Errorf("Read dir %s error %v", container.ContainerDir, err)
		return
	}

	var containerInfos []*container.ContainerInfo

	// 遍历所有文件
	for _, dir := range containerDirs {
		
		// 若获取 containernames.json ，则直接 continue
		if dir.Name() == container.ContainerNameFile {
			continue
		}
		
		// 获取 containerInfo
		tmpContainerInfo, err := container.GetContainerInfo(dir)
		if err != nil {
			log.Errorf("Get container info error %v", err)
			continue
		}

		// 检测当前 container 状态
		tmpContainerInfo.Status.StatusCheck()
		
		// 若无 -a ，则不显示 running 状态之外的 containerinfo
		if !all && !tmpContainerInfo.Status.Running {
			continue
		}
		
		containerInfos = append(containerInfos, tmpContainerInfo)
	}

	// 使用 tabwriter.NewWriter 在 终端 打出容器信息，打印对齐的表格
	w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
	fmt.Fprint(w, "CONTAINER ID\tIMAGE\tNAME\tPID\tSTATUS\tCOMMAND\tUP TIME\tCREATED\n")
	for _, info := range containerInfos {
		fmt.Fprintf( w, "%s\t%s\t%s\t%v\t%s\t%s\t%s\t%s\n",
			info.ID,
			info.Image,
			info.Name,
			info.Status.Pid,
			info.Status.Status,
			strings.Join(info.Command," "),
			// 匿名函数
			func(info *container.ContainerInfo) string {
				if info.Status.Running {
					// string => time
					startTime, err := time.ParseInLocation(
						"2006-01-02 15:04:05",
						info.Status.StartTime, 
						time.Local,
					)

					if err != nil {
						return "NULL"
					}
					
					// 当前时间
					newTime := time.Now()
					// 获取时间差
					uptime := newTime.Sub(startTime)
					
					switch{
						// up time days
						case uptime.Hours() >= 24: return fmt.Sprintf("Up %d Days", int(uptime.Hours()/24))
						// up time hours
						case uptime.Hours() < 24 && uptime.Hours() >= 1: return fmt.Sprintf("Up %d Hours", int(uptime.Hours()))
						// up time min
						case uptime.Minutes() < 60 && uptime.Minutes() >= 1: return fmt.Sprintf("Up %d Minutes", int(uptime.Minutes()))
						// up time sec
						case uptime.Seconds() < 60 : return fmt.Sprintf("Up %d Seconds", int(uptime.Seconds()))
					}
					
				}
				return "NULL"	
			} (info),
			info.CreatedTime,
		)
	}
	if err := w.Flush(); err != nil {
		log.Errorf("Flush error %v", err)
		return
	}
}
