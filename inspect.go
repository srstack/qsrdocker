package main

import (
	"fmt"
	"encoding/json"
	"strings"
	"os"
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