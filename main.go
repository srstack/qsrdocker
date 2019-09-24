package main

import (
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/urfave/cli"
)

const usage = ` qsrdocekr is a simple container runtime implementation.`

var runCmd = cli.Command{}

var initCmd = cli.Command{}

func main() {

	/*
		初始化cli
	*/
	qsrdocker := cli.NewApp()
	qsrdocker.Name = "qsrdocker"
	qsrdocker.Usage = usage

	/*
		定义cli的runCmd initCmd
	*/
	qsrdocker.Commands = []cli.Command{
		initCmd,
		runCmd,
	}

	/*
		设定log配置项
	*/
	qsrdocker.Before = func(context *cli.Context) error {
		//  log 使用 json 格式序列化
		log.SetFormatter(&log.JSONFormatter{})
		log.SetOutput(os.Stdout)

		return nil
	}

	err := qsrdocker.Run(os.Args)

	if err != nil {
		log.Fatal(err)
	}

}
