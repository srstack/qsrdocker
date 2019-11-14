package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/urfave/cli"
	"github.com/srstack/qsrdocker/container"
)

// run 命令定义函数的Flges，可使用 -- 指定参数
var runCmd = cli.Command{
	Name: "run",
	Usage: `Create a container with namespace and cgroup

		-i		Keep STDIN open even if not attached
		-t		container's stdin stdout and stderr improt bash stdin stdout and stderr`,

	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "i", // 指定 i 参数即当前的输入导入到标准输入
			Usage: `Keep STDIN open `,
		},

		cli.BoolFlag{
			Name:  "t", // 指定 ti 参数即当前的输出导入到标准输出
			Usage: `enable tty `,
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
		tty := context.Bool("i") && context.Bool("t")
		// -ti 或者 -it 都可以

		QsrdockerRun(tty, cmd)
		return nil
	},
}

/*
init 初始化函数, 该函数/操作为 runCmd 默认会调用的内部方法，禁止外部调用
*/
var initCmd = cli.Command{
	Name: "init",
	Usage: `init container process run user's process in container, Do not call it outside.
		Warring: you can not use init in bash/sh !`,

	/*
		1. 获取传递过来的 参数
		2. 执行容器初始化
	*/
	Action: func(context *cli.Context) error {
		cmd := context.Args().Get(0) // []string{"init",command}
		log.Debugf("init qsrdocker and cmd : %s", cmd)
		err := container.RunCotainerInitProcess(cmd, nil)
		return err
	},
}