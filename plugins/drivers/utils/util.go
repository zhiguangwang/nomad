package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"syscall"

	plugin "github.com/hashicorp/go-plugin"
	"github.com/hashicorp/nomad/helper/discover"
	"github.com/hashicorp/nomad/plugins/drivers/executor"
)

// isolateCommand sets the setsid flag in exec.Cmd to true so that the process
// becomes the process leader in a new session and doesn't receive signals that
// are sent to the parent process.
func isolateCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}

func GetPluginsMap() map[string]plugin.Plugin {
	e := &executor.ExecutorPlugin{
		Impl: &executor.ExecutorRPCServer{},
	}
	return map[string]plugin.Plugin{
		"executor": e,
	}
}

func CreateExecutor(w io.Writer, runtimeInfo *executor.RuntimeInfo,
	config *executor.Config) (executor.Executor, *plugin.Client, error) {

	c, err := json.Marshal(config)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create executor config: %v", err)
	}
	bin, err := discover.NomadExecutable()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to find the nomad binary: %v", err)
	}

	pluginConfig := &plugin.ClientConfig{
		Cmd:             exec.Command(bin, "executor", string(c)),
		HandshakeConfig: executor.HandshakeConfig,
		Plugins:         runtimeInfo.Plugins,
		MaxPort:         runtimeInfo.MaxPort,
		MinPort:         runtimeInfo.MinPort,
	}

	executorClient := plugin.NewClient(pluginConfig)
	rpcClient, err := executorClient.Client()
	if err != nil {
		return nil, nil, fmt.Errorf("error creating rpc client for executor plugin: %v", err)
	}

	raw, err := rpcClient.Dispense("executor")
	if err != nil {
		return nil, nil, fmt.Errorf("unable to dispense the executor plugin: %v", err)
	}

	executorPlugin := raw.(executor.Executor)
	return executorPlugin, executorClient, nil
}
