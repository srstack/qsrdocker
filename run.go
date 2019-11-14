package main

import (
	log "github.com/Sirupsen/logrus"
	"os"
	"github.com/srstack/qsrdocker/container"
)

func QsrdockerRun(tty bool, command string) {
	parent := container.NewParentProcess(tty, command)

	err := parent.Start() // 启动真正的容器进程

	if err != nil {
		log.Error(err)
	}

	parent.Wait()
	os.Exit(-1)
}