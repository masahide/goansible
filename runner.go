package goansible

import (
	"fmt"
	"sync"
	"time"
)

type RunResult struct {
	Task    *Task
	Result  *Result
	Runtime time.Duration
}

type Runner struct {
	env       *Environment
	plays     []*Play
	wait      sync.WaitGroup
	to_notify map[string]struct{}
	//async     chan *AsyncAction
	report Reporter

	Results []RunResult
	Start   time.Time
	Runtime time.Duration
}

func NewRunner(env *Environment, plays []*Play) *Runner {
	r := &Runner{
		env:       env,
		plays:     plays,
		to_notify: make(map[string]struct{}),
		//async:     make(chan *AsyncAction),
		report: env.report,
	}

	//go r.handleAsync()

	return r
}

func (r *Runner) SetReport(rep Reporter) {
	r.report = rep
}

func (r *Runner) AddNotify(n string) {
	r.to_notify[n] = struct{}{}
}

func (r *Runner) ShouldRunHandler(name string) bool {
	_, ok := r.to_notify[name]

	return ok
}

/*
func (r *Runner) AsyncChannel() chan *AsyncAction {
	return r.async
}
*/

func (r *Runner) Run(env *Environment) error {
	start := time.Now()
	r.Start = start
	var err error

	defer func() {
		r.Runtime = time.Since(start)
	}()

	r.report.StartTasks(r)

	defer r.report.EndReporter(r, err)

	for _, play := range r.plays {
		fs := NewFutureScope(play.Vars)

		for _, task := range play.Tasks {
			err = r.runTask(env, play, task, fs, fs)
			if err != nil {
				return err
			}
		}

		r.Results = append(r.Results, fs.Results()...)
	}

	r.report.FinishTasks(r)

	r.wait.Wait()

	StartHandlers := false

	for _, play := range r.plays {
		fs := NewFutureScope(play.Vars)

		for _, task := range play.Handlers {
			if r.ShouldRunHandler(task.Name()) {
				if !StartHandlers {
					r.report.StartHandlers(r)
					StartHandlers = true
				}
				err := r.runTask(env, play, task, fs, fs)

				if err != nil {
					return err
				}
			}
		}

		fs.Wait()
	}

	if StartHandlers {
		r.report.FinishHandlers(r)
	}
	r.report.FinishAll(r)

	return nil
}

func RunAdhocTask(cmd, args string) (*Result, error) {
	env := NewEnv(NewNestedScope(nil), &Config{})
	defer env.Cleanup()

	task := AdhocTask(cmd, args)

	str, err := ExpandTemplates(env.Vars, task.Args())
	if err != nil {
		return nil, err
	}

	obj, _, err := MakeCommand(env.Vars, task, str)
	if err != nil {
		return nil, err
	}

	//ar := &AdhocProgress{out: os.Stdout, Start: time.Now()}

	ce := &CommandEnv{Env: env, Paths: env.Paths /*progress: ar*/}

	return obj.Run(ce)
}

func RunAdhocTaskVars(td TaskData) (*Result, error) {
	env := NewEnv(NewNestedScope(nil), &Config{})
	defer env.Cleanup()

	task := &Task{data: td}
	task.Init(env)

	obj, _, err := MakeCommand(env.Vars, task, "")
	if err != nil {
		return nil, err
	}

	//ar := &AdhocProgress{out: os.Stdout, Start: time.Now()}

	ce := &CommandEnv{Env: env, Paths: env.Paths /*progress: ar*/}

	return obj.Run(ce)
}

func RunAdhocCommand(cmd Command, debug, output bool, args string) (*Result, error) {
	env := NewEnv(NewNestedScope(nil), &Config{ShowCommandOutput: output, Debug: debug})
	defer env.Cleanup()

	//ar := &AdhocProgress{out: os.Stdout, Start: time.Now()}

	ce := &CommandEnv{Env: env, Paths: env.Paths /*progress: ar*/}

	return cmd.Run(ce)
}

type PriorityScope struct {
	task Vars
	rest Scope
}

func (p *PriorityScope) Get(key string) (Value, bool) {
	if p.task != nil {
		if v, ok := p.task[key]; ok {
			return Any(v), true
		}
	}

	return p.rest.Get(key)
}

func (p *PriorityScope) Set(key string, val interface{}) {
	p.rest.Set(key, val)
}

func boolify(str string) bool {
	switch str {
	case "", "false", "no":
		return false
	default:
		return true
	}
}

type ModuleRun struct {
	Play        *Play
	Task        *Task
	Module      *Module
	Runner      *Runner
	Scope       Scope
	FutureScope *FutureScope
	Vars        Vars
}

func (m *ModuleRun) Run(env *CommandEnv) (*Result, error) {
	for _, task := range m.Module.ModTasks {
		ns := NewNestedScope(m.Scope)

		for k, v := range m.Vars {
			ns.Set(k, v)
		}

		err := m.Runner.runTask(env.Env, m.Play, task, ns, m.FutureScope)
		if err != nil {
			return nil, err
		}
	}

	return NewResult(true), nil
}

func (r *Runner) runTaskItems(env *Environment, play *Play, task *Task, s Scope, fs *FutureScope, start time.Time) error {

	for _, item := range task.Items() {
		ns := NewNestedScope(s)
		ns.Set("item", item)

		var res *Result

		name, err := ExpandTemplates(ns, task.Name())
		if err != nil {
			res = FailureResult(err)
			r.report.StartTask(task, task.Name(), "", nil)
			r.report.FinishTask(task, res)
			return err
		}

		str, err := ExpandTemplates(ns, task.Args())
		if err != nil {
			res = FailureResult(err)
			r.report.StartTask(task, name, "", nil)
			r.report.FinishTask(task, res)
			return err
		}

		cmd, sm, err := MakeCommand(ns, task, str)
		if err != nil {
			res = FailureResult(err)
			r.report.StartTask(task, name, str, nil)
			r.report.FinishTask(task, res)
			return err
		}

		r.report.StartTask(task, name, str, sm)

		ce := NewCommandEnv(env, task)

		res, err = cmd.Run(ce)
		if err != nil && res == nil {
			res = FailureResult(err)
		}

		if name := task.Register(); name != "" {
			res.Data.Set("Changed", res.Changed)
			res.Data.Set("Failed", res.Failed)
			fs.Set(name, res)
		}

		runtime := time.Since(start)

		r.Results = append(r.Results, RunResult{task, res, runtime})

		r.report.FinishTask(task, res)

		if err == nil {
			for _, x := range task.Notify() {
				r.AddNotify(x)
			}
		} else {
			return err
		}
	}

	return nil
}

func (r *Runner) runTask(env *Environment, play *Play, task *Task, s Scope, fs *FutureScope) error {
	ps := &PriorityScope{task.IncludeVars, s}

	start := time.Now()

	if when := task.When(); when != "" {
		when, err := ExpandVars(ps, when)

		if err != nil {
			return err
		}

		if !boolify(when) {
			return nil
		}
	}

	env.Vars = s
	if items := task.Items(); items != nil {
		return r.runTaskItems(env, play, task, s, fs, start)
	}

	name, err := ExpandTemplates(ps, task.Name())
	if err != nil {
		return err
	}

	str, err := ExpandTemplates(ps, task.Args())
	if err != nil {
		return err
	}

	var cmd Command

	var argVars Vars

	if mod, ok := play.Modules[task.Command()]; ok {
		sm, err := ParseSimpleMap(s, str)
		if err != nil {
			return err
		}

		for ik, iv := range task.Vars {
			if str, ok := iv.Read().(string); ok {
				exp, err := ExpandTemplates(s, str)
				if err != nil {
					return err
				}

				sm[ik] = Any(exp)
			} else {
				sm[ik] = iv
			}
		}

		cmd = &ModuleRun{
			Play:        play,
			Task:        task,
			Module:      mod,
			Runner:      r,
			Scope:       s,
			FutureScope: NewFutureScope(s),
			Vars:        sm,
		}

		argVars = sm
	} else {
		cmd, argVars, err = MakeCommand(ps, task, str)

		if err != nil {
			return err
		}
	}

	r.report.StartTask(task, name, str, argVars)

	ce := NewCommandEnv(env, task)

	if name := task.Future(); name != "" {
		future := NewFuture(start, task, func() (*Result, error) {
			return cmd.Run(ce)
		})

		fs.AddFuture(name, future)

		return nil
	}

	/*
		if task.Async() {
			asyncAction := &AsyncAction{Task: task}
			asyncAction.Init(r)

			go func() {
				asyncAction.Finish(cmd.Run(ce))
			}()
		} else { */
	res, err := cmd.Run(ce)
	if err != nil && res == nil {
		res = FailureResult(err)
	}

	if name := task.Register(); name != "" {
		res.Data.Set("changed", fmt.Sprintf("%v", res.Changed))
		res.Data.Set("failed", fmt.Sprintf("%v", res.Failed))
		fs.Set(name, res)
	}

	runtime := time.Since(start)

	r.Results = append(r.Results, RunResult{task, res, runtime})

	r.report.FinishTask(task, res)

	if err == nil && res.Changed {
		for _, x := range task.Notify() {
			r.AddNotify(x)
		}
	} else {
		return err
	}
	//}

	return err
}
