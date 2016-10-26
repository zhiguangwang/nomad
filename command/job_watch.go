package command

import (
	"fmt"
	"strings"

	ui "github.com/gizak/termui"
	"github.com/hashicorp/nomad/api"
)

type JobWatchCommand struct {
	Meta
	config *JobWatchConfig
	client *api.Client
}

type JobWatchConfig struct {
	// JobID is the job to watch
	JobID string

	// ExitAfter causes the watch to be automatically exited in the passed
	// seconds unless the user intervenes
	ExitAfter int
}

func (c *JobWatchCommand) Help() string {
	helpText := `
Usage: nomad job watch [options] <job>


General Options:

  ` + generalOptionsUsage() + `

Job Watch Options:

  -exit-after=<seconds>
	Causes the watch to automatically exit after the given number of seconds
	unless the exit is cancelled.

  -eval=<eval-id>
    Constrain watching allocations from the most recent evaluation.
`
	return strings.TrimSpace(helpText)
}

func (c *JobWatchCommand) Synopsis() string {
	return "Watch a job for scheduling and task related events."
}

func (c *JobWatchCommand) Run(args []string) int {
	var exitAfter int

	flags := c.Meta.FlagSet("watch", FlagSetClient)
	flags.Usage = func() { c.Ui.Output(c.Help()) }
	flags.IntVar(&exitAfter, "exit-after", 0, "")

	if err := flags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Parsing flags failed: %v", err))
		return 1
	}

	c.config = &JobWatchConfig{
		ExitAfter: exitAfter,
	}

	// Get the HTTP client
	client, err := c.Meta.Client()
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error initializing client: %s", err))
		return 1
	}
	c.client = client

	return c.run()
}

func (c *JobWatchCommand) run() int {
	err := ui.Init()
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Initializing UI failed: %v", err))
		return 1
	}
	defer ui.Close()

	// Add the exit gauge
	if c.config.ExitAfter != 0 {
		c.addExitAfterGauge()
	}

	ls := ui.NewList()
	ls.Border = false
	ls.Items = []string{
		"[1] Downloading File 1",
		"", // == \newline
		"[2] Downloading File 2",
		"",
		"[3] Uploading File 3",
	}
	ls.Height = 5

	par := ui.NewPar("<> This row has 3 columns\n<- Widgets can be stacked up like left side\n<- Stacked widgets are treated as a single widget")
	par.Height = 5
	par.BorderLabel = "Demonstration"

	// build layout
	ui.Body.AddRows(
		ui.NewRow(
			ui.NewCol(3, 0, ls),
			ui.NewCol(6, 0, par)))

	// calculate layout
	ui.Body.Align()

	ui.Render(ui.Body)

	ui.Handle("/sys/kbd/q", func(ui.Event) {
		ui.StopLoop()
	})

	ui.Handle("/sys/wnd/resize", func(e ui.Event) {
		ui.Body.Width = ui.TermWidth()
		ui.Body.Align()
		ui.Clear()
		ui.Render(ui.Body)
	})

	ui.Loop()
	return 0
}

func (c *JobWatchCommand) addExitAfterGauge() {
	exitGauge := ui.NewGauge()
	exitGauge.BorderLabel = "Automatically exiting - Press \"c\" to cancel"
	exitGauge.Height = 3
	exitGauge.Border = true
	exitGauge.Percent = 100
	exitGauge.BarColor = ui.ColorRed
	ui.Body.AddRows(ui.NewCol(0, 0, exitGauge))

	exitGauge.Handle("/timer/1s", func(e ui.Event) {
		t := e.Data.(ui.EvtTimer)
		i := t.Count
		if int(i) == c.config.ExitAfter && exitGauge.Display != false {
			ui.StopLoop()
			return
		}

		exitGauge.Percent = int(float64(c.config.ExitAfter-int(i)) / float64(c.config.ExitAfter) * 100)
		ui.Render(ui.Body)
	})

	exitGauge.Handle("/sys/kbd/c", func(ui.Event) {
		if exitGauge.Display {
			exitGauge.Display = false
			ui.Body.Rows = ui.Body.Rows[1:]
			ui.Body.Align()
			ui.Clear()
			ui.Render(ui.Body)
		}
	})
}

func (c *JobWatchCommand) jobStatus() (ui.GridBufferer, bool, int) {
	// Try querying the job
	jobID := c.config.JobID
	jobs, _, err := c.client.Jobs().PrefixList(jobID)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error querying job: %s", err))
		return nil, true, 1
	}
	if len(jobs) == 0 {
		c.Ui.Error(fmt.Sprintf("No job(s) with prefix or id %q found", jobID))
		return nil, true, 1
	}
	if len(jobs) > 1 && strings.TrimSpace(jobID) != jobs[0].ID {
		c.Ui.Output(fmt.Sprintf("Prefix matched multiple jobs\n\n%s", createStatusListOutput(jobs)))
		return nil, true, 0
	}
	// Prefix lookup matched a single job
	job, _, err := c.client.Jobs().Info(jobs[0].ID, nil)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error querying job: %s", err))
		return nil, true, 1
	}

	return nil, false, 0
}
