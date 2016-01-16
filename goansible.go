package goansible

/*
import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var cUpdateScript = []byte(`#!/bin/bash

cd .goansible

REL=$TACHYON_RELEASE
BIN=goansible-$TACHYON_OS-$TACHYON_ARCH

if test -f goansible; then
  CUR=$(< release)
  if test "$REL" != "$CUR"; then
    echo "Detected goansible of old release ($CUR), removing."
    rm goansible
  fi
fi

if which curl > /dev/null; then
  DL="curl -O"
elif which wget > /dev/null; then
  DL="wget"
else
  echo "No curl or wget, unable to pull a release"
  exit 1
fi

if ! test -f goansible; then
  echo "Downloading $REL/$BIN..."

  $DL https://s3-us-west-2.amazonaws.com/goansible.vektra.io/$REL/sums
  if which gpg > /dev/null; then
    gpg --keyserver keys.gnupg.net --recv-key A408199F &
    $DL https://s3-us-west-2.amazonaws.com/goansible.vektra.io/$REL/sums.asc &
  fi

  $DL https://s3-us-west-2.amazonaws.com/goansible.vektra.io/$REL/$BIN

  wait

  if which gpg > /dev/null; then
    if ! gpg --verify sums.asc; then
      echo "Signature verification failed! Aborting!"
      exit 1
    fi
  fi

  mv $BIN $BIN.gz

  # If gunzip fails, it's because the file isn't gzip'd, so we
  # assume it's already in the correct format.
  if ! gunzip $BIN.gz; then
    mv $BIN.gz $BIN
  fi

  if which shasum > /dev/null; then
    if ! (grep $BIN sums | shasum -c); then
      echo "Sum verification failed!"
      exit 1
    fi
  else
    echo "No shasum available to verify files"
  fi

  echo $REL > release

  chmod a+x $BIN
  ln -s $BIN goansible
fi
`)

func normalizeArch(arch string) string {
	switch arch {
	case "x86_64":
		return "amd64"
	default:
		return arch
	}
}

type Tachyon struct {
	Target      string `goansible:"target"`
	Debug       bool   `goansible:"debug"`
	Output      bool   `goansible:"output"`
	Clean       bool   `goansible:"clean"`
	Dev         bool   `goansible:"dev"`
	Playbook    string `goansible:"playbook"`
	Release     string `goansible:"release"`
	NoJSON      bool   `goansible:"no_json"`
	InstallOnly bool   `goansible:"install_only"`
}

func (t *Tachyon) ShowOutput() bool { return t.Output }
func (t *Tachyon) ShowDebug() bool  { return t.Debug }

func (t *Tachyon) Run(env *CommandEnv) (*Result, error) {
	if t.Release == "" {
		t.Release = Release
	}

	ssh := NewSSH(t.Target)
	ssh.Debug = t.Debug

	defer ssh.Cleanup()

	// err := ssh.Start()
	// if err != nil {
	// return nil, fmt.Errorf("Error starting persistent SSH connection: %s\n", err)
	// }

	var bootstrap string

	if t.Clean {
		bootstrap = "rm -rf .goansible && mkdir -p .goansible"
	} else {
		bootstrap = "mkdir -p .goansible"
	}

	out, err := ssh.RunAndCapture(bootstrap + " && uname && uname -m")
	if err != nil {
		return nil, fmt.Errorf("Error creating remote .goansible dir: %s (%s)", err, string(out))
	}

	tos, arch, ok := split2(string(out), "\n")
	if !ok {
		return nil, fmt.Errorf("Unable to figure out os and arch of remote machine\n")
	}

	tos = strings.ToLower(tos)
	arch = normalizeArch(strings.TrimSpace(arch))

	binary := fmt.Sprintf("goansible-%s-%s", tos, arch)

	if t.Dev {
		env.Progress("Copying development goansible...")

		path := filepath.Dir(Arg0)

		err = ssh.CopyToHost(filepath.Join(path, binary), ".goansible/"+binary+".new")
		if err != nil {
			return nil, fmt.Errorf("Error copying goansible to vagrant: %s\n", err)
		}

		ssh.Run(fmt.Sprintf("cd .goansible && mv %[1]s.new %[1]s && ln -fs %[1]s goansible", binary))
	} else {
		env.Progress("Updating goansible release...")

		c := ssh.Command("cat > .goansible/update && chmod a+x .goansible/update")

		c.Stdout = os.Stdout
		c.Stdin = bytes.NewReader(cUpdateScript)
		err = c.Run()
		if err != nil {
			return nil, fmt.Errorf("Error updating, well, the updater: %s\n", err)
		}

		cmd := fmt.Sprintf("TACHYON_RELEASE=%s TACHYON_OS=%s TACHYON_ARCH=%s ./.goansible/update", t.Release, tos, arch)
		err = ssh.Run(cmd)
		if err != nil {
			return nil, fmt.Errorf("Error running updater: %s", err)
		}
	}

	if t.InstallOnly {
		res := NewResult(true)
		res.Add("target", t.Target)
		res.Add("install_only", true)

		return res, nil
	}

	var src string

	var main string

	fi, err := os.Stat(t.Playbook)
	if err != nil {
		return nil, err
	}

	if fi.IsDir() {
		src, err = filepath.Abs(t.Playbook)
		if err != nil {
			return nil, fmt.Errorf("Unable to resolve %s: %s", t.Playbook, err)
		}
		main = "site.yml"
	} else {
		abs, err := filepath.Abs(t.Playbook)
		if err != nil {
			return nil, fmt.Errorf("Unable to resolve %s: %s", t.Playbook, err)
		}

		main = filepath.Base(abs)
		src = filepath.Dir(abs)
	}

	src += "/"

	env.Progress("Syncing playbook...")

	c := exec.Command("rsync", "-av", "-e", ssh.RsyncCommand(), src, ssh.Host+":.goansible/playbook")

	if t.Debug {
		c.Stdout = os.Stdout
	}

	err = c.Run()

	if err != nil {
		return nil, fmt.Errorf("Error copying playbook to vagrant: %s\n", err)
	}

	env.Progress("Running playbook...")

	var format string

	if !t.NoJSON {
		format = "--json"
	}

	startCmd := fmt.Sprintf("cd .goansible && sudo ./goansible %s playbook/%s", format, main)

	c = ssh.Command(startCmd)

	if t.Debug {
		fmt.Fprintf(os.Stderr, "Run: %#v\n", c.Args)
	}

	stream, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}

	c.Stderr = os.Stderr

	c.Start()

	input := bufio.NewReader(stream)

	for {
		str, err := input.ReadString('\n')
		if err != nil {
			break
		}

		sz, err := strconv.Atoi(strings.TrimSpace(str))
		if err != nil {
			break
		}

		data := make([]byte, sz)

		_, err = input.Read(data)
		if err != nil {
			break
		}

		_, err = input.ReadByte()
		if err != nil {
			break
		}

		env.progress.JSONProgress(data)
	}

	if err != nil {
		if err != io.EOF {
			fmt.Printf("error: %s\n", err)
		}
	}

	err = c.Wait()
	if err != nil {
		return nil, fmt.Errorf("Error running playbook remotely: %s", err)
	}

	res := NewResult(true)
	res.Add("target", t.Target)
	res.Add("playbook", t.Playbook)

	return res, nil
}

func init() {
	RegisterCommand("goansible", &Tachyon{})
}
*/
