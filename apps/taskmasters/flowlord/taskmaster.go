package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"time"

	"github.com/pcelvng/task"
	"github.com/pcelvng/task-tools/bootstrap"
	"github.com/pcelvng/task-tools/file"
	"github.com/pcelvng/task-tools/slack"
	"github.com/pcelvng/task-tools/tmpl"
	"github.com/pcelvng/task-tools/workflow"
	"github.com/pcelvng/task/bus"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
)

type taskMaster struct {
	initTime    time.Time
	path        string
	dur         time.Duration
	producer    bus.Producer
	consumer    bus.Consumer
	fOpts       *file.Options
	doneTopic   string
	failedTopic string
	*workflow.Cache
	cron  *cron.Cron
	slack *slack.Slack
}

type stats struct {
	RunTime string       `json:"runtime"`
	Entries []cron.Entry `json:"entries"`
}

func New(app *bootstrap.TaskMaster) bootstrap.Runner {
	opts := app.AppOpt().(*options)
	bOpts := app.GetBusOpts()
	bOpts.InTopic = opts.DoneTopic
	if bOpts.Bus == "pubsub" {
		bOpts.InChannel = opts.DoneTopic + "-flowlord"
	}
	consumer, err := bus.NewConsumer(bOpts)
	if err != nil {
		log.Fatal("consumer init", err)
	}
	return &taskMaster{
		initTime:    time.Now(),
		path:        opts.Workflow,
		doneTopic:   opts.DoneTopic,
		failedTopic: opts.FailedTopic,
		fOpts:       opts.File,
		producer:    app.NewProducer(),
		consumer:    consumer,
		cron:        cron.New(cron.WithSeconds()),
		dur:         opts.Refresh,
		slack:       opts.Slack,
	}
}

func (tm *taskMaster) Info() interface{} {
	return stats{
		RunTime: time.Now().Sub(tm.initTime).String(),
		Entries: tm.cron.Entries(),
	}
}

// AutoUpdate will create a go routine to auto update the cached files
// if any changes have been made to the workflow files
func (tm *taskMaster) AutoUpdate() {
	for {
		files, err := tm.Cache.Refresh()
		if err != nil {
			log.Println("error reloading workflow files", err)
			return
		}
		// if there are values in files, there are changes that need to be reloaded
		if len(files) > 0 {
			log.Println("reloading workflow changes")
			tcron := tm.cron
			tm.cron = cron.New(cron.WithSeconds())
			if err := tm.schedule(); err != nil {
				log.Println("error setting up cron schedule", err)
				tm.cron = tcron
			} else {
				tcron.Stop()
			}
		}
		<-time.Tick(tm.dur)
	}
}

func (tm *taskMaster) Run(ctx context.Context) (err error) {
	if tm.Cache, err = workflow.New(tm.path, tm.fOpts); err != nil {
		return errors.Wrapf(err, "workflow setup")
	}

	// refresh the workflow if the file(s) have been changed
	go tm.AutoUpdate()

	if err := tm.schedule(); err != nil {
		return errors.Wrapf(err, "cron schedule")
	}

	go tm.read(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return nil
		}
	}
}

// schedule the tasks and refresh the schedule when updated
func (tm *taskMaster) schedule() (err error) {
	if len(tm.Workflows) == 0 {
		return fmt.Errorf("no workflows found check path %s", tm.path)
	}
	for path, workflow := range tm.Workflows {
		for _, w := range workflow.Parent() {
			rules, _ := url.ParseQuery(w.Rule)
			if rules.Get("cron") == "" {
				log.Printf("skip: task:%s, rule:%s", w.Task, w.Rule)
				continue
			}

			j := &job{
				Name:     rules.Get("job"),
				Workflow: path,
				Topic:    w.Task,
				Schedule: rules.Get("cron"),
				Template: w.Template,
				producer: tm.producer,
			}
			if s := rules.Get("offset"); s != "" {
				j.Offset, err = time.ParseDuration(s)
				if err != nil {
					return errors.Wrapf(err, "invalid duration %s", s)
				}
			}

			if _, err = tm.cron.AddJob(j.Schedule, j); err != nil {
				return errors.Wrapf(err, "invalid rule for %s:%s %s", path, w.Task, w.Rule)
			}
			log.Printf("cron: task:%s, rule:%s, info:%s", w.Task, j.Schedule, w.Template)
			log.Println(rules)
		}
	}
	tm.cron.Start()
	return nil
}

// Process the given task
// 1. check if the task needs to be retried
// 2. start any downstream tasks
// Send retry failed tasks to tm.failedTopic
func (tm *taskMaster) Process(t *task.Task) error {
	meta, _ := url.ParseQuery(t.Meta)
	// attempt to return
	if t.Result == task.ErrResult {
		w := tm.Get(*t)
		r := meta.Get("retry")
		i, _ := strconv.Atoi(r)
		if w.Retry > i {
			t = task.NewWithID(t.Type, t.Info, t.ID)
			i++
			meta.Set("retry", strconv.Itoa(i))
			t.Meta = meta.Encode()
			if err := tm.producer.Send(t.Type, t.JSONBytes()); err != nil {
				return err
			}
		} else if tm.failedTopic != "-" {
			// send to the retry failed topic if retries > w.Retry
			meta.Set("retry", "failed")
			t.Meta = meta.Encode()
			tm.producer.Send(tm.failedTopic, t.JSONBytes())
			if tm.slack != nil {
				b, _ := json.MarshalIndent(t, "", "  ")
				tm.slack.Notify(string(b), slack.Critical)
			}
		}
		return nil
	}

	// start off any children tasks
	if t.Result == task.CompleteResult {
		for _, w := range tm.Children(*t) {
			info := tmpl.Meta(w.Template, meta)
			child := task.NewWithID(w.Task, info, t.ID)
			child.Meta = "workflow=" + meta.Get("workflow")
			if err := tm.producer.Send(w.Task, child.JSONBytes()); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("unknown result %q %s", t.Result, t.JSONString())
}

func (tm *taskMaster) read(ctx context.Context) {
	for {
		b, done, err := tm.consumer.Msg()
		if done || task.IsDone(ctx) {
			log.Println("stopping consumer")
			return
		}
		if err != nil {
			log.Println("consumer", err)
			return
		}
		t := &task.Task{}
		if err = json.Unmarshal(b, t); err != nil {
			log.Printf("unmarshal error %q: %s", string(b), err)
			continue
		}
		if err := tm.Process(t); err != nil {
			log.Println(err)
		}
	}
}
