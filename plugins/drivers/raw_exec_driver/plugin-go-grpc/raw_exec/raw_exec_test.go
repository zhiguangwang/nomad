package raw_exec

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/hashicorp/nomad/helper/testtask"
	"github.com/hashicorp/nomad/plugins/drivers/structs"
	"github.com/stretchr/testify/require"
)

// Test that RawExec can execute a simple task that writes to a file
func TestRawExecDriver_Start(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	execCtx := &structs.ExecContext{
		TaskDir: &structs.TaskDir{},
	}

	file := "output.txt"
	allocDir := "NOMAD_ALLOC_DIR"

	writeValue := "hello world"
	outPath := fmt.Sprintf(`${%s}/%s`, allocDir, file)
	taskInfo := &structs.TaskInfo{
		Config: &RawExecConfig{
			Command: testtask.Path(),
			Args: []string{
				"sleep",
				"1s",
				"write",
				writeValue,
				outPath,
			},
		},
	}

	d := NewRawExecDriver()
	resp, err := d.Start(execCtx, taskInfo)
	require.Nil(err)
	require.NotNil(resp)

	sharedDir := ""
	outputFile := filepath.Join(sharedDir, file)
	out, err := ioutil.ReadFile(outputFile)
	require.Nil(err)
	require.Equal(out, []byte(writeValue))
}
