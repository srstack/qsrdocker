package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/srstack/qsrdocker/container"
	"github.com/urfave/cli"
)

const usage = ` qsrdocekr is a simple container runtime implementation.`

// run 命令定义函数的Flges，可使用 -- 指定参数
var runCmd = cli.Command{
	Name: "run",
	Usage: `Create a container with namespace and cgroup .
			qsrdocker run -ti [command]
			-ti container's stdin stdout and stderr improt bash stdin stdout and stderr
			`,

	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "ti", // 指定 ti 参数即当前的输入输出导入到标准输入输出
			Usage: `enable tty`,
		},
	},

	/*
		1. 是否包含 cmd
		2. 获取用户指定 cmd
		3. 调用 run 函数
	*/
	Action: func(context *cli.Context) error {

		if len(context.Args()) < 1 {
			return fmt.Errorf("miss cmd")
		}

		cmd := context.Args().Get(0)
		tty := context.Bool("ti")
		qsrdockerRun(tty, cmd)
		return nil
	},
}

/*
init 初始化函数, 该函数/操作为 runCmd 默认会调用的内部方法，禁止外部调用
*/
var initCmd = cli.Command{
	Name: "init",
	Usage: `init container process run user's process in container, Do not call it outside .
			warring: you can not use init in bash/sh !
			`,

	/*
		1. 获取传递过来的 参数
		2. 执行容器初始化
	*/
	Action: func(context *cli.Context) error {
		cmd := context.Args().Get(0) // []string{"init", command}
		log.Infof("init qsrdocker cmd : %s", cmd)
		err := container.RunCotainerInitProcess(cmd, nil)
		return err
	},
}

func main() {

	/*
		初始化cli
	*/
	qsrdocker := cli.NewApp()
	qsrdocker.Name = "qsrdocker"
	qsrdocker.UsageText = usage
	qsrdocker.Usage = usage
	qsrdocker.Version = "0.0.01"

	// 定义cli的runCmd initCmd
	qsrdocker.Commands = []cli.Command{
		initCmd,
		runCmd,
	}

	// 设定log配置项
	qsrdocker.Before = func(context *cli.Context) error {
		//  log 使用 json 格式序列化
		log.SetFormatter(&log.JSONFormatter{})
		log.SetOutput(os.Stdout)

		return nil
	}

	err := qsrdocker.Run(os.Args) // 获取输入 os.Args，该输入在 cil 操作中以 context 体现/控制

	if err != nil {
		log.Fatal(err)
	}

}

func qsrdockerRun(tty bool, command string) {
	parent := container.NewParentProcess(tty, command)

	err := parent.Start() // 启动真正的容器进程

	if err != nil {
		log.Error(err)
	}

	parent.Wait()
	os.Exit(-1)
}
