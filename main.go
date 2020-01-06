package main

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const usage = ` qsrdoceker is a simple container runtime implementation.`

func main() {
	/*
		初始化cli
	*/
	qsrdocker := cli.NewApp()
	qsrdocker.Name = "qsrdocker"
	qsrdocker.UsageText = usage
	qsrdocker.Usage = usage
	qsrdocker.Version = "1.0.0"

	// 定义cli的runCmd initCmd
	qsrdocker.Commands = []cli.Command{
		initCmd,
		runCmd,
		commitCmd,
		listCmd,
		logCmd,
		execCmd,
		inspectCmd,
		stopCmd,
		removeCmd,
	}

	// 设定log配置项
	qsrdocker.Before = func(context *cli.Context) error {
		//  log 使用 json 格式序列化
		log.SetFormatter(&log.JSONFormatter{})
		log.SetOutput(os.Stdout)

		//log.SetLevel(log.WarnLevel)
		log.SetLevel(log.DebugLevel)
		return nil
	}

	err := qsrdocker.Run(os.Args) // 获取输入 os.Args，该输入在 cil 操作中以 context 体现/控制

	if err != nil {
		log.Fatal(err)
	}

}
