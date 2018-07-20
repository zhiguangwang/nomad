package raw_exec

import (
	"fmt"
	"os"

	"github.com/hashicorp/nomad/plugins/drivers/executor"
	"github.com/hashicorp/nomad/plugins/drivers/structs"
	"github.com/hashicorp/nomad/plugins/drivers/utils"
)

type RawExec struct {
}

// RawExecConfig implements the Config interface
type RawExecConfig struct {
	Command string
	Args    []string
}

func (c *RawExecConfig) DriverName() string {
	return "Raw Exec"
}

// TODO do we need to inject a driver context? Probably not as drivers are now not 1:1 with tasks
func NewRawExecDriver() *RawExec {
	return &RawExec{}
}

func (*RawExec) Start(ctx *structs.ExecContext, task *structs.TaskInfo) (*structs.StartResponse, error) {
	if ctx == nil || task == nil {
		return nil, fmt.Errorf("ExecContext or TaskInfo should not be nil")
	}
	if task.Config == nil {
		return nil, fmt.Errorf("Task Config should not be nil")
	}
	//command := task.Config.Command
	executorConfig := &executor.Config{}
	runtimeInfo := &executor.RuntimeInfo{
		Plugins: utils.GetPluginsMap(),
	}

	logOutput := os.Stderr
	exec, pluginClient, err := utils.CreateExecutor(logOutput, runtimeInfo, executorConfig)
	if err != nil {
		return nil, fmt.Errorf("Unable to create plugin client: %s", err.Error())
	}

	executorCtx := &executor.ExecutorContext{
		TaskEnv: ctx.TaskEnv,
		Task:    task,
		Driver:  "raw_exec",
		TaskDir: ctx.TaskDir.Directory,
		LogDir:  ctx.TaskDir.LogDir,
	}

	if err := exec.SetContext(executorCtx); err != nil {
		pluginClient.Kill()
		return nil, fmt.Errorf("failed to set executor context: %v", err)
	}

	res := &structs.StartResponse{TaskId: "12345"}
	return res, nil
}
