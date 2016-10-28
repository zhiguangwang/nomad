package command

import (
	"fmt"
	"strings"
	"time"

	ui "github.com/gizak/termui"
	"github.com/hashicorp/nomad/api"
)

type JobWatchCommand struct {
	Meta
	config *JobWatchConfig
	client *api.Client

	// err holds an error that occured that should be displayed to the user
	err error

	// jobStopped marks a job as being stopped
	jobStopped bool
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

	// Try querying the job
	jobID := c.config.JobID
	jobs, _, err := c.client.Jobs().PrefixList(jobID)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error querying job: %s", err))
		return 1
	}
	if len(jobs) == 0 {
		c.Ui.Error(fmt.Sprintf("No job(s) with prefix or id %q found", jobID))
		return 1
	}
	if len(jobs) > 1 && strings.TrimSpace(jobID) != jobs[0].ID {
		c.Ui.Output(fmt.Sprintf("Prefix matched multiple jobs\n\n%s", createStatusListOutput(jobs)))
		return 0
	}
	// Prefix lookup matched a single job
	job, _, err := c.client.Jobs().Info(jobs[0].ID, nil)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error querying job: %s", err))
		return 1
	}

	return c.run(job)
}

func (c *JobWatchCommand) run(job *api.Job) int {
	err := ui.Init()
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Initializing UI failed: %v", err))
		return 1
	}
	defer func() {
		// Clear and close
		ui.Close()

		if c.err != nil {
			c.Ui.Error(fmt.Sprintf("Exiting on error: %v", err))
		} else if c.jobStopped {
			c.Ui.Output(fmt.Sprintf("Job %q was stopped", job.ID))
		}
	}()

	// Start the watchers
	go c.pollJob(job)

	// Add the exit gauge
	if c.config.ExitAfter != 0 {
		c.addExitAfterGauge()
	}

	// Get the job status
	status := NewJobStatusGrid(job)

	// build layout
	ui.Body.AddRows(
		ui.NewRow(ui.NewCol(6, 0, status)),
	)

	// calculate layout
	ui.Body.Align()

	ui.Render(ui.Body)

	ui.Handle("/sys/kbd", func(ui.Event) {
		ui.StopLoop()
	})

	ui.Handle("/sys/kbd/c", func(ui.Event) {
	})

	ui.Handle("/sys/wnd/resize", func(e ui.Event) {
		ui.Body.Width = ui.TermWidth()
		ui.Body.Align()
		ui.Clear()
		ui.Render(ui.Body)
	})

	ui.Handle("/error", func(e ui.Event) {
		err, ok := e.Data.(error)
		if !ok {
			panic(e.Data)
		}

		c.err = err
		ui.StopLoop()
	})

	ui.Handle("/job/stopped", func(ui.Event) {
		c.jobStopped = true
		ui.StopLoop()
	})

	ui.Loop()

	if c.err != nil {
		return 1
	}

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
			exitGauge.Height = 0
			exitGauge.Width = 0
			exitGauge.Border = false
			ui.Body.Align()
			ui.Clear()
			ui.Render(ui.Body)
		}
	})
}

func (c *JobWatchCommand) pollJob(job *api.Job) {
	q := &api.QueryOptions{
		WaitIndex: job.ModifyIndex,
	}
	for {
		updated, _, err := c.client.Jobs().Info(job.ID, q)
		if err != nil {
			if strings.Contains(err.Error(), "job not found") {
				ui.SendCustomEvt("/job/stopped", fmt.Errorf("Failed to poll job: %v", err))
			} else {
				ui.SendCustomEvt("/watch/job", fmt.Errorf("Failed to poll job: %v", err))
			}
			return
		}

		if updated != nil {
			ui.SendCustomEvt("/watch/job", updated)
			q.WaitIndex = updated.ModifyIndex
		}
	}
}

type JobStatusGrid struct {
	*ui.Par
	job *api.Job
}

func NewJobStatusGrid(job *api.Job) *JobStatusGrid {
	js := &JobStatusGrid{
		job: job,
		Par: ui.NewPar(""),
	}
	js.Height = 8
	js.Border = true
	js.BorderLabel = "Job Info"
	js.render()

	// Handle updates
	js.Handle("/watch/job", func(e ui.Event) {
		job, ok := e.Data.(*api.Job)
		if !ok {
			panic(e.Data)
		}

		js.job = job
		js.render()
		ui.Body.Align()
		ui.Clear()
		ui.Render(ui.Body)
	})

	return js
}

func (js *JobStatusGrid) Update(job *api.Job) {
	js.job = job
	js.render()
}

func (js *JobStatusGrid) render() {
	// Check if it is periodic
	sJob, err := convertApiJob(js.job)
	if err != nil {
		ui.SendCustomEvt("/watch/job", fmt.Errorf("Failed to poll job: %v", err))
		return
	}
	periodic := sJob.IsPeriodic()

	// Format the job info
	basic := []string{
		fmt.Sprintf("ID|%s", js.job.ID),
		fmt.Sprintf("Name|%s", js.job.Name),
		fmt.Sprintf("Type|%s", js.job.Type),
		fmt.Sprintf("Priority|%d", js.job.Priority),
		fmt.Sprintf("Datacenters|%s", strings.Join(js.job.Datacenters, ",")),
		fmt.Sprintf("Status|%s", js.job.Status),
		fmt.Sprintf("Periodic|%v", periodic),
	}

	if periodic {
		now := time.Now().UTC()
		next := sJob.Periodic.Next(now)
		basic = append(basic, fmt.Sprintf("Next Periodic Launch|%s",
			fmt.Sprintf("%s (%s from now)",
				formatTime(next), formatTimeDifference(now, next, time.Second))))
	}

	js.Text = formatKV(basic)
}
