package command

import (
	"fmt"
	"sort"
	"strings"
	"time"

	ui "github.com/gizak/termui"
	"github.com/hashicorp/nomad/api"
)

const (
	numRecentEvals  = 5
	numRecentAllocs = 5
	numRecentEvents = 15
)

type JobWatchCommand struct {
	Meta
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

	args = flags.Args()
	if len(args) > 1 {
		c.Ui.Error(c.Help())
		return 1
	}

	w := &JobWatcher{
		Meta: c.Meta,
		Config: &JobWatchConfig{
			JobID:     args[0],
			ExitAfter: exitAfter,
		},
	}

	return w.Run()
}

type JobWatcher struct {
	Meta

	Config *JobWatchConfig

	client *api.Client

	// err holds an error that occured that should be displayed to the user
	err error

	// jobStopped marks a job as being stopped
	jobStopped bool
}

func (c *JobWatcher) Run() int {
	// Get the HTTP client
	client, err := c.Meta.Client()
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error initializing client: %s", err))
		return 1
	}
	c.client = client

	// Try querying the job
	jobID := c.Config.JobID
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

	if err := ui.Init(); err != nil {
		c.Ui.Error(fmt.Sprintf("Initializing UI failed: %v", err))
		return 1
	}
	defer func() {
		// Clear and close
		ui.Close()

		if c.err != nil {
			c.Ui.Error(fmt.Sprintf("Exiting on error: %v", c.err))
		} else if c.jobStopped {
			c.Ui.Output(fmt.Sprintf("Job %q was stopped", job.ID))
		}
	}()

	// Start the watchers
	go c.pollJob(job)
	go c.pollJobSummary(job)
	go c.pollEvals(job)
	go c.pollAllocations(job)

	// Add the exit gauge
	if c.Config.ExitAfter != 0 {
		c.addExitAfterGauge()
	}

	// Get the job status and summary
	status := NewJobStatusGrid(job)
	summary := NewJobSummarysGrid()

	// Watch for scheduling events
	// TODO verbose mode
	latestEvals := NewLatestEvalsGrid(numRecentEvals, 8)
	placementFailures := NewLatestEvalFailureGrid(8)

	// Watch for allocation updates
	latestAllocs := NewAllocUpdatesGrid(numRecentAllocs, 8)
	latestTaskEvents := NewTaskEventsGrid(numRecentEvents, 8)

	// Build layout
	ui.Body.AddRows(
		ui.NewRow(
			ui.NewCol(3, 0, status),
			ui.NewCol(9, 0, summary),
		),
		ui.NewRow(
			ui.NewCol(6, 0, latestEvals),
			ui.NewCol(6, 0, placementFailures),
		),
		ui.NewRow(
			ui.NewCol(6, 0, latestAllocs),
			ui.NewCol(6, 0, latestTaskEvents),
		),
		//ui.NewRow(
		//ui.NewCol(12, 0, latestTaskEvents),
		//),
	)

	// Calculate layout
	ui.Body.Align()
	ui.Render(ui.Body)

	ui.Handle("/sys/kbd/q", func(ui.Event) {
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

func (c *JobWatcher) addExitAfterGauge() {
	exitGauge := ui.NewGauge()
	exitGauge.BorderLabel = " Automatically exiting - Press \"c\" to cancel "
	exitGauge.Height = 3
	exitGauge.Border = true
	exitGauge.Percent = 100
	exitGauge.BarColor = ui.ColorRed
	ui.Body.AddRows(ui.NewCol(0, 0, exitGauge))

	exitGauge.Handle("/timer/1s", func(e ui.Event) {
		t := e.Data.(ui.EvtTimer)
		i := t.Count
		if int(i) == c.Config.ExitAfter && exitGauge.Display != false {
			ui.StopLoop()
			return
		}

		exitGauge.Percent = int(float64(c.Config.ExitAfter-int(i)) / float64(c.Config.ExitAfter) * 100)
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

func (c *JobWatcher) pollJob(job *api.Job) {
	q := &api.QueryOptions{
		WaitIndex: job.ModifyIndex,
	}
	for {
		updated, _, err := c.client.Jobs().Info(job.ID, q)
		if err != nil {
			if strings.Contains(err.Error(), "job not found") {
				ui.SendCustomEvt("/job/stopped", fmt.Errorf("Failed to poll job: %v", err))
			} else {
				ui.SendCustomEvt("/error", fmt.Errorf("Failed to poll job: %v", err))
			}
			return
		}

		if updated != nil {
			ui.SendCustomEvt("/watch/job/info", updated)
			q.WaitIndex = updated.ModifyIndex
		}
	}
}

func (c *JobWatcher) pollJobSummary(job *api.Job) {
	q := &api.QueryOptions{
		WaitIndex: 0,
	}
	for {
		updated, _, err := c.client.Jobs().Summary(job.ID, q)
		if err != nil {
			if strings.Contains(err.Error(), "job not found") {
				ui.SendCustomEvt("/job/stopped", fmt.Errorf("Failed to poll job: %v", err))
			} else {
				ui.SendCustomEvt("/error", fmt.Errorf("Failed to poll job summary: %v", err))
			}
			return
		}

		if updated != nil {
			ui.SendCustomEvt("/watch/job/summary", updated)
			q.WaitIndex = updated.ModifyIndex
		}
	}
}

func (c *JobWatcher) pollEvals(job *api.Job) {
	q := &api.QueryOptions{
		WaitIndex: 0,
	}
	for {
		updated, meta, err := c.client.Jobs().Evaluations(job.ID, q)
		if err != nil {
			if strings.Contains(err.Error(), "job not found") {
				ui.SendCustomEvt("/job/stopped", fmt.Errorf("Failed to poll job: %v", err))
			} else {
				ui.SendCustomEvt("/error", fmt.Errorf("Failed to poll job summary: %v", err))
			}
			return
		}

		if updated != nil {
			ui.SendCustomEvt("/watch/job/evals", updated)
			q.WaitIndex = meta.LastIndex
		}
	}
}

func (c *JobWatcher) pollAllocations(job *api.Job) {
	q := &api.QueryOptions{
		WaitIndex: 0,
	}
	for {
		updated, meta, err := c.client.Jobs().Allocations(job.ID, q)
		if err != nil {
			if strings.Contains(err.Error(), "job not found") {
				ui.SendCustomEvt("/job/stopped", fmt.Errorf("Failed to poll job: %v", err))
			} else {
				ui.SendCustomEvt("/error", fmt.Errorf("Failed to poll job allocations: %v", err))
			}
			return
		}

		if updated != nil {
			ui.SendCustomEvt("/watch/job/allocs", updated)
			q.WaitIndex = meta.LastIndex
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
	js.Border = true
	js.BorderLabel = " Job Info "
	js.PaddingLeft = 1
	js.render()

	// Handle updates
	js.Handle("/watch/job/info", func(e ui.Event) {
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

func (js *JobStatusGrid) render() {
	// Check if it is periodic
	sJob, err := convertApiJob(js.job)
	if err != nil {
		ui.SendCustomEvt("/error", fmt.Errorf("Failed to convert api job: %v", err))
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

	js.Height = len(basic) + 2
	js.Text = formatKV(basic)
}

type JobSummaryGrid struct {
	*ui.Par
	summary *api.JobSummary
}

func NewJobSummarysGrid() *JobSummaryGrid {
	js := &JobSummaryGrid{
		Par: ui.NewPar(""),
	}
	js.Height = 8
	js.Border = true
	js.BorderLabel = " Job Summary "
	js.PaddingLeft = 1
	js.render()

	// Handle updates
	js.Handle("/watch/job/summary", func(e ui.Event) {
		summary, ok := e.Data.(*api.JobSummary)
		if !ok {
			panic(e.Data)
		}

		js.summary = summary
		js.render()
		ui.Body.Align()
		ui.Clear()
		ui.Render(ui.Body)
	})

	return js
}

func (js *JobSummaryGrid) render() {
	if js.summary == nil {
		js.Text = ""
		return
	}

	summaries := make([]string, len(js.summary.Summary)+1)
	summaries[0] = "Task Group|Queued|Starting|Running|Failed|Complete|Lost"
	taskGroups := make([]string, 0, len(js.summary.Summary))
	for taskGroup := range js.summary.Summary {
		taskGroups = append(taskGroups, taskGroup)
	}
	sort.Strings(taskGroups)
	for idx, taskGroup := range taskGroups {
		tgs := js.summary.Summary[taskGroup]
		summaries[idx+1] = fmt.Sprintf("%s|%d|%d|%d|%d|%d|%d",
			taskGroup, tgs.Queued, tgs.Starting,
			tgs.Running, tgs.Failed,
			tgs.Complete, tgs.Lost,
		)
	}

	js.Height = len(summaries) + 2
	js.Text = formatList(summaries)
}

type LatestEvalsGrid struct {
	*ui.Par
	limit  int
	length int
	evals  []*api.Evaluation
}

func NewLatestEvalsGrid(limit, length int) *LatestEvalsGrid {
	l := &LatestEvalsGrid{
		Par:    ui.NewPar(""),
		limit:  limit,
		length: length,
	}
	l.Height = 8
	l.Border = true
	l.BorderLabel = " Recent Scheduling Events "
	l.PaddingLeft = 1
	l.render()

	// Handle updates
	l.Handle("/watch/job/evals", func(e ui.Event) {
		evals, ok := e.Data.([]*api.Evaluation)
		if !ok {
			panic(e.Data)
		}

		l.evals = evals
		l.render()
		ui.Body.Align()
		ui.Clear()
		ui.Render(ui.Body)
	})

	return l
}

func (l *LatestEvalsGrid) render() {
	if l.evals == nil {
		l.Text = ""
		return
	}

	numEvals := len(l.evals)
	if numEvals > l.limit {
		numEvals = l.limit
	}

	// Format the evals
	evals := make([]string, numEvals+1)
	evals[0] = "ID|Triggered By|Status|Failures"
	for i, eval := range l.evals[:numEvals] {
		failures, _ := evalFailureStatus(eval)
		evals[i+1] = fmt.Sprintf("%s|%s|%s|%s",
			limit(eval.ID, l.length),
			eval.TriggeredBy,
			eval.Status,
			failures,
		)
	}

	l.Height = len(evals) + 2
	l.Text = formatList(evals)
}

type LatestEvalFailureGrid struct {
	*ui.Par
	length int
	evals  []*api.Evaluation
}

func NewLatestEvalFailureGrid(length int) *LatestEvalFailureGrid {
	l := &LatestEvalFailureGrid{
		Par:    ui.NewPar(""),
		length: length,
	}
	l.Height = 8
	l.Border = true
	l.BorderLabel = " Placement Failures "
	l.PaddingLeft = 1
	l.render()

	// Handle updates
	l.Handle("/watch/job/evals", func(e ui.Event) {
		evals, ok := e.Data.([]*api.Evaluation)
		if !ok {
			panic(e.Data)
		}

		l.evals = evals
		l.render()
		ui.Body.Align()
		ui.Clear()
		ui.Render(ui.Body)
	})

	return l
}

func (l *LatestEvalFailureGrid) render() {
	if l.evals == nil {
		l.Text = ""
		return
	}

	// Determine latest evaluation with failures whose follow up hasn't
	// completed
	failedEval := latestFailedEval(l.evals)
	if failedEval != nil {
		text := ""
		sorted := sortedTaskGroupFromMetrics(failedEval.FailedTGAllocs)
		for i, tg := range sorted {
			if i >= maxFailedTGs {
				break
			}

			text += fmt.Sprintf("Task Group %q:\n", tg)
			metrics := failedEval.FailedTGAllocs[tg]
			text += formatAllocMetrics(metrics, false, "  ") + "\n"
			if i != len(sorted)-1 {
				text += "\n"
			}
		}

		if len(sorted) > maxFailedTGs {
			text += fmt.Sprintf("\nPlacement failures truncated. To see remainder run:\nnomad eval-status %s\n", failedEval.ID)
		}

		l.Height = 2 + len(strings.Split(text, "\n"))
		l.Text = strings.TrimSpace(text)
	} else {
		l.Height = 3
		l.Text = "No Placement Failures"
	}
}

// TODO back port
func latestFailedEval(evals []*api.Evaluation) *api.Evaluation {
	// Find the latest good eval and whether there is a blocked eval
	var latestGoodEval uint64
	blocked := false
	for _, eval := range evals {
		if eval.Status == "complete" && len(eval.FailedTGAllocs) == 0 {
			if eval.CreateIndex > latestGoodEval {
				latestGoodEval = eval.CreateIndex
			}
		}

		if eval.Status == "blocked" {
			blocked = true
		}
	}

	// Find the latest failed eval
	var latestFailedEval *api.Evaluation
	for _, eval := range evals {
		if eval.Status == "complete" && len(eval.FailedTGAllocs) != 0 {
			if latestFailedEval == nil || eval.CreateIndex > latestFailedEval.CreateIndex {
				latestFailedEval = eval
			}
		}
	}

	if latestFailedEval == nil || latestFailedEval.CreateIndex < latestGoodEval || !blocked {
		return nil
	}

	return latestFailedEval
}

type AllocUpdatesGrid struct {
	*ui.Par
	allocs []*api.AllocationListStub
	limit  int
	length int
}

func NewAllocUpdatesGrid(limit, length int) *AllocUpdatesGrid {
	au := &AllocUpdatesGrid{
		Par:    ui.NewPar(""),
		limit:  limit,
		length: length,
	}
	au.Height = 8
	au.Border = true
	au.BorderLabel = " Recent Allocation Changes "
	au.PaddingLeft = 1
	au.render()

	// Handle updates
	au.Handle("/watch/job/allocs", func(e ui.Event) {
		allocs, ok := e.Data.([]*api.AllocationListStub)
		if !ok {
			panic(e.Data)
		}

		au.allocs = allocs
		au.render()
		ui.Body.Align()
		ui.Clear()
		ui.Render(ui.Body)
	})

	return au
}

func (au *AllocUpdatesGrid) render() {
	if au.allocs == nil {
		au.Text = ""
		return
	}

	numAllocs := len(au.allocs)
	if numAllocs > au.limit {
		numAllocs = au.limit
	}

	sortAllocListStubByModifyIndex(au.allocs)

	allocs := make([]string, numAllocs+1)
	allocs[0] = "ID|Node ID|Task Group|Desired|Status|Created At"
	for idx, alloc := range au.allocs[:numAllocs] {
		created := formatTime(time.Unix(0, alloc.CreateTime))
		allocs[idx+1] = fmt.Sprintf("%s|%s|%s|%s|%s|%s",
			limit(alloc.ID, au.length),
			limit(alloc.NodeID, au.length),
			alloc.TaskGroup,
			alloc.DesiredStatus,
			alloc.ClientStatus,
			created,
		)
	}

	au.Height = len(allocs) + 2
	au.Text = formatList(allocs)
}

type modifyIndexSorter []*api.AllocationListStub

func (m modifyIndexSorter) Len() int { return len(m) }

func (m modifyIndexSorter) Less(i, j int) bool {
	a, b := m[i], m[j]
	if a.ModifyIndex < b.ModifyIndex {
		return false
	} else if a.ModifyIndex == b.ModifyIndex {
		return a.CreateIndex > b.CreateIndex
	} else {
		return true
	}
}

func (m modifyIndexSorter) Swap(i, j int) { m[i], m[j] = m[j], m[i] }

func sortAllocListStubByModifyIndex(allocs []*api.AllocationListStub) {
	m := modifyIndexSorter(allocs)
	sort.Sort(m)
}

type TaskEventsGrid struct {
	*ui.Par
	allocs []*api.AllocationListStub
	limit  int
	length int
}

func NewTaskEventsGrid(limit, length int) *TaskEventsGrid {
	t := &TaskEventsGrid{
		Par:    ui.NewPar(""),
		limit:  limit,
		length: length,
	}
	t.Height = 8
	t.Border = true
	t.BorderLabel = " Recent Task Events "
	t.PaddingLeft = 1
	t.render()

	// Handle updates
	t.Handle("/watch/job/allocs", func(e ui.Event) {
		allocs, ok := e.Data.([]*api.AllocationListStub)
		if !ok {
			panic(e.Data)
		}

		t.allocs = allocs
		t.render()
		ui.Body.Align()
		ui.Clear()
		ui.Render(ui.Body)
	})

	return t
}

func (t *TaskEventsGrid) render() {
	if t.allocs == nil {
		t.Text = ""
		return
	}

	recent := mostRecentTaskEvents(t.allocs, t.limit)

	events := make([]string, len(recent)+1)
	events[0] = "ID|Task|Group|State|Event|Description|Time"
	for idx, w := range recent {
		created := formatTime(time.Unix(0, w.event.Time))
		events[idx+1] = fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s",
			limit(w.allocID, t.length),
			w.task,
			w.taskgroup,
			w.state.State,
			w.event.Type,
			limitElipses(w.event.Description(), int(float64(t.Width)*0.5)),
			created)
	}

	t.Height = len(events) + 2
	t.Text = formatList(events)
}

type taskEventWrapper struct {
	state     *api.TaskState
	event     *api.TaskEvent
	allocID   string
	taskgroup string
	task      string
}

func mostRecentTaskEvents(allocs []*api.AllocationListStub, limit int) []taskEventWrapper {
	events := make([]taskEventWrapper, limit)

	for _, alloc := range allocs {
		for task, state := range alloc.TaskStates {
			//for _, e := range state.Events {
			for j := len(state.Events) - 1; j >= 0; j-- {
				e := state.Events[j]
				insertPoint := sort.Search(limit, func(i int) bool {
					w := events[i]
					// Hasn't been set yet
					if w.event == nil {
						return true
					}

					if e.Time > w.event.Time {
						return true
					}

					return false
				})

				if insertPoint >= limit {
					continue
				}

				events[insertPoint].event = e
				events[insertPoint].state = state
				events[insertPoint].allocID = alloc.ID
				events[insertPoint].taskgroup = alloc.TaskGroup
				events[insertPoint].task = task
			}
		}
	}

	lastSet := limit - 1
	for {
		if events[lastSet].event == nil {
			lastSet--
		} else {
			break
		}
	}

	return events[:lastSet]
}

func limitElipses(s string, cutoff int) string {
	oLength := len(s)
	if oLength < cutoff {
		return s
	}
	if cutoff < 3 {
		return "..."
	}

	return fmt.Sprintf("%s...", limit(s, cutoff-3))
}
