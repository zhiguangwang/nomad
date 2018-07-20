package executor

import (
	"log"
	"net/rpc"
	"time"

	"github.com/hashicorp/go-plugin"
)

// Registering these types since we have to serialize and de-serialize the Task
// structs over the wire between drivers and the executor.
func init() {
}

type ExecutorRPC struct {
	client *rpc.Client
}

// LaunchCmdArgs wraps a user command and the args for the purposes of RPC
type LaunchCmdArgs struct {
	Cmd *ExecCommand
}

type ExecCmdArgs struct {
	Deadline time.Time
	Name     string
	Args     []string
}

type ExecCmdReturn struct {
	Output []byte
	Code   int
}

func (e *ExecutorRPC) ShutDown() error {
	return e.client.Call("Plugin.ShutDown", new(interface{}), new(interface{}))
}

func (e *ExecutorRPC) Exit() error {
	return e.client.Call("Plugin.Exit", new(interface{}), new(interface{}))
}

func (e *ExecutorRPC) SetContext(ctx *ExecutorContext) error {
	return e.client.Call("Plugin.SetContext", ctx, new(interface{}))
}

func (e *ExecutorRPC) Exec(deadline time.Time, name string, args []string) ([]byte, int, error) {
	req := ExecCmdArgs{
		Deadline: deadline,
		Name:     name,
		Args:     args,
	}
	var resp *ExecCmdReturn
	err := e.client.Call("Plugin.Exec", req, &resp)
	if resp == nil {
		return nil, 0, err
	}
	return resp.Output, resp.Code, err
}

type ExecutorRPCServer struct {
	Impl   Executor
	logger *log.Logger
}

func (e *ExecutorRPCServer) ShutDown(args interface{}, resp *interface{}) error {
	return e.Impl.ShutDown()
}

func (e *ExecutorRPCServer) Exit(args interface{}, resp *interface{}) error {
	return e.Impl.Exit()
}

func (e *ExecutorRPCServer) SetContext(args *ExecutorContext, resp *interface{}) error {
	return e.Impl.SetContext(args)
}

func (e *ExecutorRPCServer) Exec(args ExecCmdArgs, result *ExecCmdReturn) error {
	out, code, err := e.Impl.Exec(args.Deadline, args.Name, args.Args)
	ret := &ExecCmdReturn{
		Output: out,
		Code:   code,
	}
	*result = *ret
	return err
}

type ExecutorPlugin struct {
	logger *log.Logger
	Impl   *ExecutorRPCServer
}

func (p *ExecutorPlugin) Server(*plugin.MuxBroker) (interface{}, error) {
	if p.Impl == nil {
		p.Impl = &ExecutorRPCServer{Impl: DefaultExecutor{}}
	}
	return p.Impl, nil
}

func (p *ExecutorPlugin) Client(b *plugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &ExecutorRPC{client: c}, nil
}
