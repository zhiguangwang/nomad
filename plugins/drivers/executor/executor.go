package executor

import (
	"os"
	"time"

	"github.com/hashicorp/go-plugin"
	structs "github.com/hashicorp/nomad/plugins/drivers/structs"
)

var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "NOMAD_PLUGIN_MAGIC_COOKIE",
	MagicCookieValue: "e4327c2e01eabfd75a8a67adb114fb34a757d57eee7728d857a8cec6e91a7255",
}

type RuntimeInfo struct {
	Plugins map[string]plugin.Plugin
	MaxPort uint
	MinPort uint
}

type Config struct {
	Logfile  string
	Loglevel string
}

// ExecCommand holds the user command, args, and other isolation related
// settings.
type ExecCommand struct {
	// Cmd is the command that the user wants to run.
	Cmd string

	// Args is the args of the command that the user wants to run.
	Args []string

	// TaskKillSignal is an optional field which signal to kill the process
	TaskKillSignal os.Signal

	// FSIsolation determines whether the command would be run in a chroot.
	FSIsolation bool

	// User is the user which the executor uses to run the command.
	User string

	// ResourceLimits determines whether resource limits are enforced by the
	// executor.
	ResourceLimits bool

	// Cgroup marks whether we put the process in a cgroup. Setting this field
	// doesn't enforce resource limits. To enforce limits, set ResourceLimits.
	// Using the cgroup does allow more precise cleanup of processes.
	BasicProcessCgroup bool
}

// ProcessState holds information about the state of a user process.
type ProcessState struct {
	Pid      int
	ExitCode int
	Signal   int
	Time     time.Time
}

// ExecutorContext holds context to configure the command user
// wants to run and isolate it
type ExecutorContext struct {
	// TaskEnv holds information about the environment of a Task
	TaskEnv *structs.TaskEnv

	Task *structs.TaskInfo

	// TaskDir is the host path to the task's root
	TaskDir string

	// LogDir is the host path where logs should be written
	LogDir string

	// Driver is the name of the driver that invoked the executor
	Driver string

	// PortUpperBound is the upper bound of the ports that we can use to start
	// the syslog server
	PortUpperBound uint

	// PortLowerBound is the lower bound of the ports that we can use to start
	// the syslog server
	PortLowerBound uint
}

// Executor is the interface which allows a driver to launch and supervise
// a process
type Executor interface {
	SetContext(*ExecutorContext) error
	ShutDown() error
	Exit() error
	Exec(deadline time.Time, cmd string, args []string) ([]byte, int, error)
}

type DefaultExecutor struct {
	ctx *ExecutorContext
}

func (e DefaultExecutor) SetContext(ctx *ExecutorContext) error {
	e.ctx = ctx
	return nil
}

func (e DefaultExecutor) ShutDown() error {
	return nil
}

func (e DefaultExecutor) Exit() error {
	return nil
}

func (e DefaultExecutor) Exec(deadline time.Time, cmd string, args []string) ([]byte, int, error) {
	return []byte{}, 0, nil
}
