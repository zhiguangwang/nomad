package structs

type TaskDir struct {
	Directory      string
	SharedAllocDir string
	SharedTaskDir  string
	LocalDir       string
	LogDir         string
	SecretsDir     string
}

type TaskEnv struct {
	NodeAttrs map[string]string
	EnvMap    map[string]string
}

type ExecContext struct {
	TaskDir *TaskDir
	TaskEnv *TaskEnv
}

type Resources struct{}
type LogConfig struct{}

type Config interface {
	DriverName() string
}

type TaskInfo struct {
	Resources *Resources
	LogConfig *LogConfig
	Config    Config
}

type StartResponse struct {
	TaskId string
}
