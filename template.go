package goansible

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
)

type TemplateCmd struct {
	Src   string `goansible:"src,required"`
	Dest  string `goansible:"dest,required"`
	Owner string `goansible:"owner"`
	Uid   int    `goansible:"uid"`
	Gid   int    `goansible:"gid"`
	Mode  int    `goansible:"mode"`
}

func (cmd *TemplateCmd) Run(env *CommandEnv) (*Result, error) {
	var src string
	//pp.Print(env)
	if cmd.Mode == 0 {
		cmd.Mode = 0644
	}

	if cmd.Src[0] == '/' {
		src = cmd.Src
	} else {
		src = env.Paths.TemplateFile(cmd.Src)
	}

	//input, err := os.Open(src)
	b, err := ioutil.ReadFile(src)

	if err != nil {
		return FailureResult(err), err
	}

	expand, err := ExpandTemplates(env.Env.Vars, string(b))
	if err != nil {
		return FailureResult(err), err
	}

	srcDigest := md5string(expand)

	var dstDigest []byte

	dest := cmd.Dest

	link := false

	destStat, err := os.Lstat(dest)
	if err == nil {
		if destStat.IsDir() {
			dest = filepath.Join(dest, filepath.Base(src))
		} else {
			dstDigest, _ = md5file(dest)
		}

		link = destStat.Mode()&os.ModeSymlink != 0
	}

	rd := ResultData{
		"md5sum": Any(hex.EncodeToString(srcDigest)),
		"src":    Any(src),
		"dest":   Any(dest),
	}

	//pp.Print(dstDigest, srcDigest)
	if dstDigest != nil && bytes.Equal(srcDigest, dstDigest) {
		changed := false

		if cmd.Mode != 0 && destStat.Mode() != os.FileMode(cmd.Mode) {
			changed = true
			if err := os.Chmod(dest, os.FileMode(cmd.Mode)); err != nil {
				return FailureResult(err), err
			}
		}
		if cmd.Uid, cmd.Gid, err = ChangePerm(cmd.Owner, cmd.Uid, cmd.Gid); err != nil {
			return FailureResult(err), err
		}
		if estat, ok := destStat.Sys().(*syscall.Stat_t); ok {
			if cmd.Uid != int(estat.Uid) || cmd.Gid != int(estat.Gid) {
				changed = true
				os.Chown(dest, cmd.Uid, cmd.Gid)
			}
		}

		return WrapResult(changed, rd), nil
	}

	tmp := fmt.Sprintf("%s.tmp.%d", cmd.Dest, os.Getpid())

	//	output, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0644)
	err = ioutil.WriteFile(tmp, []byte(expand), 0644)

	if err != nil {
		return FailureResult(err), err
	}

	if link {
		os.Remove(dest)
	}

	if err := os.Chmod(tmp, os.FileMode(cmd.Mode)); err != nil {
		os.Remove(tmp)
		return FailureResult(err), err
	}

	if cmd.Uid, cmd.Gid, err = ChangePerm(cmd.Owner, cmd.Uid, cmd.Gid); err != nil {
		return FailureResult(err), err
	}
	os.Chown(tmp, cmd.Uid, cmd.Gid)

	err = os.Rename(tmp, dest)
	if err != nil {
		os.Remove(tmp)
		return FailureResult(err), err
	}

	return WrapResult(true, rd), nil
}

func init() {
	RegisterCommand("template", &TemplateCmd{})
}
