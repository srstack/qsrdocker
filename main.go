package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/srstack/qsrdocker/container"
	"github.com/urfave/cli"
)

const usage = ` qsrdocekr is a simple container runtime implementation.`

// run 命令定义函数
var runCmd = cli.Command{
	Name:  "run",
	Usage: `Create a container with namespace and cgroup`,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "ti",
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
		Run(tty, cmd)
		return nil
	},
}

/*
init 初始化函数
*/
var initCmd = cli.Command{
	Name:  "init",
	Usage: `init container process run user's process in container, Do not call it outside`,

	/*
		1. 获取传递过来的 参数
		2. 执行容器初始化
	*/

	Action: func(context *cli.Context) error {
		log.Infof("init qsrdocker")
		cmd := context.Args().Get(0)
		log.Infof("init cmd : %s", cmd)
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

func Run(tty bool, command string) {
	parent := container.NewParentProcess(tty, command)

	err := parent.Start()
	if err != nil {
		log.Error(err)
	}

	parent.Wait()
	os.Exit(-1)
}
