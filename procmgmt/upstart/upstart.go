package upstart

import (
	"fmt"
	"github.com/masahide/goansible"
	us "github.com/masahide/goansible/upstart"
	"io/ioutil"
	"path/filepath"
	"strings"
)

type Install struct {
	Name string `goansible:"name"`
	File string `goansible:"file"`
}

func (d *Install) Run(env *goansible.CommandEnv) (*goansible.Result, error) {
	dest := filepath.Join("/etc/init", d.Name+".conf")

	cpy := &goansible.CopyCmd{
		Src:  d.File,
		Dest: dest,
	}

	res, err := cpy.Run(env)
	if err != nil {
		return nil, err
	}

	res.Add("name", d.Name)

	return res, nil
}

type Daemon struct {
	Name       string            `goansible:"name"`
	Command    string            `goansible:"command"`
	Foreground bool              `goansible:"foreground"`
	OneFork    bool              `goansible:"one_fork"`
	Instance   string            `goansible:"instance"`
	PreStart   string            `goansible:"pre_start"`
	PostStart  string            `goansible:"post_start"`
	PreStop    string            `goansible:"pre_stop"`
	PostStop   string            `goansible:"post_stop"`
	Env        map[string]string `goansible:"env"`
}

func setScript(env *goansible.CommandEnv, code *us.Code, val string) error {
	if val == "" {
		return nil
	}

	if val[0] == '@' {
		body, err := ioutil.ReadFile(env.Paths.File(val[1:]))
		if err != nil {
			return err
		}

		code.Script = us.Script(body)
	} else {
		code.Script = us.Script(val)
	}

	return nil
}

func (d *Daemon) Run(env *goansible.CommandEnv) (*goansible.Result, error) {
	cfg := us.DaemonConfig(d.Name, d.Command)
	cfg.Env = d.Env

	if d.Foreground {
		cfg.Foreground()
	}

	if d.OneFork {
		cfg.Expect = "fork"
	}

	err := setScript(env, &cfg.PreStart, d.PreStart)
	if err != nil {
		return nil, err
	}

	err = setScript(env, &cfg.PostStart, d.PostStart)
	if err != nil {
		return nil, err
	}

	err = setScript(env, &cfg.PreStop, d.PreStop)
	if err != nil {
		return nil, err
	}

	err = setScript(env, &cfg.PostStop, d.PostStop)
	if err != nil {
		return nil, err
	}

	cfg.Instance = d.Instance

	err = cfg.Install()
	if err != nil {
		return nil, err
	}

	res := goansible.NewResult(true)
	res.Add("name", d.Name)

	return res, nil
}

type Task struct {
	Name      string            `goansible:"name"`
	Command   string            `goansible:"command"`
	Instance  string            `goansible:"instance"`
	PreStart  string            `goansible:"pre_start"`
	PostStart string            `goansible:"post_start"`
	PreStop   string            `goansible:"pre_stop"`
	PostStop  string            `goansible:"post_stop"`
	Env       map[string]string `goansible:"env"`
}

func (t *Task) Run(env *goansible.CommandEnv) (*goansible.Result, error) {
	cfg := us.TaskConfig(t.Name, t.Command)
	cfg.Env = t.Env

	cfg.Instance = t.Instance

	err := setScript(env, &cfg.PreStart, t.PreStart)
	if err != nil {
		return nil, err
	}

	err = setScript(env, &cfg.PostStart, t.PostStart)
	if err != nil {
		return nil, err
	}

	err = setScript(env, &cfg.PreStop, t.PreStop)
	if err != nil {
		return nil, err
	}

	err = setScript(env, &cfg.PostStop, t.PostStop)
	if err != nil {
		return nil, err
	}

	err = cfg.Install()
	if err != nil {
		return nil, err
	}

	res := goansible.NewResult(true)
	res.Add("name", t.Name)

	return res, nil
}

type Restart struct {
	Name string `goansible:"name"`
}

func (r *Restart) Run(env *goansible.CommandEnv) (*goansible.Result, error) {
	conn, err := us.Dial()
	if err != nil {
		return nil, err
	}

	job, err := conn.Job(r.Name)
	if err != nil {
		return nil, err
	}

	inst, err := job.Restart()
	if err != nil {
		return nil, err
	}

	pid, err := inst.Pid()
	if err != nil {
		return nil, err
	}

	res := goansible.NewResult(true)
	res.Add("name", r.Name)
	res.Add("pid", pid)

	return res, nil
}

type Stop struct {
	Name string `goansible:"name"`
}

func (r *Stop) Run(env *goansible.CommandEnv) (*goansible.Result, error) {
	conn, err := us.Dial()
	if err != nil {
		return nil, err
	}

	job, err := conn.Job(r.Name)
	if err != nil {
		return nil, err
	}

	err = job.Stop()
	if err != nil {
		if strings.Index(err.Error(), "Unknown instance") == 0 {
			res := goansible.NewResult(false)
			res.Add("name", r.Name)

			return res, nil
		}
	}

	res := goansible.NewResult(true)
	res.Add("name", r.Name)

	return res, nil
}

type Start struct {
	Name string            `goansible:"name"`
	Env  map[string]string `goansible:"env"`
}

func (r *Start) Run(env *goansible.CommandEnv) (*goansible.Result, error) {
	conn, err := us.Dial()
	if err != nil {
		return nil, err
	}

	var ienv []string

	for k, v := range r.Env {
		ienv = append(ienv, fmt.Sprintf("%s=%s", k, v))
	}

	job, err := conn.Job(r.Name)
	if err != nil {
		return nil, err
	}

	inst, err := job.StartWithOptions(ienv, true)
	if err != nil {
		if strings.Index(err.Error(), "Job is already running") == 0 {
			res := goansible.NewResult(false)
			res.Add("name", r.Name)

			return res, nil
		}
		return nil, err
	}

	pid, err := inst.Pid()
	if err != nil {
		return nil, err
	}

	res := goansible.NewResult(true)
	res.Add("name", r.Name)
	res.Add("pid", pid)

	return res, nil
}

func init() {
	goansible.RegisterCommand("upstart/install", &Install{})
	goansible.RegisterCommand("upstart/daemon", &Daemon{})
	goansible.RegisterCommand("upstart/task", &Task{})
	goansible.RegisterCommand("upstart/restart", &Restart{})
	goansible.RegisterCommand("upstart/stop", &Stop{})
	goansible.RegisterCommand("upstart/start", &Start{})
}
