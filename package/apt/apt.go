package apt

import (
	"fmt"
	"github.com/masahide/goansible"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

type Apt struct {
	Pkg       string `goansible:"pkg"`
	State     string `goansible:"state" enum:"present,install,absent,remove"`
	Cache     bool   `goansible:"update_cache"`
	CacheTime string `goansible:"cache_time"`
	Dry       bool   `goansible:"dryrun"`
}

var installed = regexp.MustCompile(`Installed: ([^\n]+)`)
var candidate = regexp.MustCompile(`Candidate: ([^\n]+)`)

func (a *Apt) Run(env *goansible.CommandEnv) (*goansible.Result, error) {
	state := a.State
	if state == "" {
		state = "present"
	}

	if a.Cache {
		home, err := goansible.HomeDir()
		if err != nil {
			return nil, err
		}

		checkFile := filepath.Join(home, ".goansible", "apt-cache-timestamp")

		runUpdate := true

		if a.CacheTime != "" {
			fi, err := os.Stat(checkFile)
			if err == nil {
				dur, err := time.ParseDuration(a.CacheTime)
				if err != nil {
					return nil, fmt.Errorf("cache_time was not in the proper format")
				}

				runUpdate = time.Now().After(fi.ModTime().Add(dur))
			}
		}

		if runUpdate {
			_, err := goansible.RunCommand(env, "apt-get", "update")
			if err != nil {
				return nil, err
			}
			ioutil.WriteFile(checkFile, []byte(``), 0666)
		}
	}

	if a.Pkg == "" {
		simp := goansible.NewResult(true)
		simp.Add("cache", "updated")

		return simp, nil
	}

	out, err := goansible.RunCommand(env, "apt-cache", "policy", a.Pkg)
	if err != nil {
		return nil, err
	}

	res := installed.FindSubmatch(out.Stdout)
	if res == nil {
		return nil, fmt.Errorf("No package '%s' available", a.Pkg)
	}

	curVer := string(res[1])
	if curVer == "(none)" {
		curVer = ""
	}

	res = candidate.FindSubmatch(out.Stdout)
	if res == nil {
		return nil, fmt.Errorf("Error parsing apt-cache output")
	}

	canVer := string(res[1])

	if state == "absent" {
		rd := goansible.ResultData{}

		if curVer == "" {
			return goansible.WrapResult(false, rd), nil
		}

		rd.Set("removed", curVer)

		_, err = goansible.RunCommand(env, "apt-get", "remove", "-y", a.Pkg)

		if err != nil {
			return nil, err
		}

		return goansible.WrapResult(true, rd), nil
	}

	rd := goansible.ResultData{}
	rd.Set("installed", curVer)
	rd.Set("candidate", canVer)

	if state == "present" && curVer == canVer {
		return goansible.WrapResult(false, rd), nil
	}

	if a.Dry {
		rd.Set("dryrun", true)
		return goansible.WrapResult(true, rd), nil
	}

	e := append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "DEBIAN_PRIORITY=critical")

	_, err = goansible.RunCommandInEnv(env, e, "apt-get", "install", "-y", a.Pkg)
	if err != nil {
		return nil, err
	}

	rd.Set("installed", canVer)

	return goansible.WrapResult(true, rd), nil
}

func init() {
	goansible.RegisterCommand("apt", &Apt{})
}
