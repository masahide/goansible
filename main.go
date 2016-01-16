package goansible

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jessevdk/go-flags"
)

type Options struct {
	Vars        map[string]string `short:"s" long:"set" description:"Set a variable"`
	ShowOutput  bool              `short:"o" long:"output" description:"Show command output"`
	ShowVersion bool              `short:"v" long:"version" description:"Show version"`
	Passphrase  string            `short:"p" long:"Passphrase" description:"passphrase of decode"`
	Development bool              `long:"dev" description:"Use a dev version of goansible"`
	Debug       bool              `short:"d" long:"debug" description:"Show all information about commands"`
	JSON        bool              `long:"json" description:"Output the run details in chunked json"`
	//CleanHost   bool              `long:"clean-host" description:"Clean the host cache before using"`
	//Host        string            `short:"t" long:"host" description:"Run the playbook on another host"`
	//Release     string            `long:"release" description:"The release to use when remotely invoking goansible"`
	//Install     bool              `long:"install" description:"Install goansible a remote machine"`
}

var Release string = "dev"
var Version string
var Arg0 string

//go:generate go-bindata -pkg=goansible data/...

func Main(args []string) int {
	var opts Options

	abs, err := filepath.Abs(args[0])
	if err != nil {
		panic(err)
	}

	Arg0 = abs

	parser := flags.NewParser(&opts, flags.Default)

	for _, o := range parser.Command.Group.Groups()[0].Options() {
		if o.LongName == "release" {
			o.Default = []string{Release}
		}
	}
	args, err = parser.ParseArgs(args)

	if err != nil {
		if serr, ok := err.(*flags.Error); ok {
			if serr.Type == flags.ErrHelp {
				return 2
			}
		}

		fmt.Printf("Error parsing options: %s", err)
		return 1
	}

	if opts.ShowVersion {
		fmt.Println(filepath.Base(args[0]), "Version:", Version)
		return 0
	}
	if /*!opts.Install &&*/ len(args) != 2 {
		fmt.Printf("Usage: goansible [options] <playbook>\n")
		return 1
	}

	/*
		if opts.Host != "" {
			return runOnHost(&opts, args)
		}
	*/

	cfg := &Config{ShowCommandOutput: opts.ShowOutput, Passphrase: opts.Passphrase}

	ns := NewNestedScope(nil)

	for k, v := range opts.Vars {
		ns.Set(k, v)
	}

	env := NewEnv(ns, cfg)
	defer env.Cleanup()

	if opts.JSON {
		//env.ReportJSON()
		env.ReportStruct()
	}

	playbookFile := args[1]
	err = StartPlaybook(env, playbookFile)
	if opts.JSON {
		j, err := json.MarshalIndent(env.report, "", " ")
		if err != nil {
			log.Println(err)
		}
		fmt.Println(string(j))
	}
	if err != nil {
		log.Println(err)
		return 1
	}
	return 0
}

func StartPlaybook(env *Environment, file string) error {

	playbook, err := NewPlaybook(env, file)
	if err != nil {
		return fmt.Errorf("Error loading plays: %s\n", err)
	}

	cur, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Unable to figure out the current directory: %s\n", err)
	}

	defer os.Chdir(cur)
	os.Chdir(playbook.baseDir)

	runner := NewRunner(env, playbook.Plays)
	err = runner.Run(env)

	if err != nil {
		return fmt.Errorf("Error running playbook: %s\n", err)
	}
	return nil
}

/*
func runOnHost(opts *Options, args []string) int {
	if opts.Install {
		fmt.Printf("=== Installing goansible on %s\n", opts.Host)
	} else {
		fmt.Printf("=== Executing playbook on %s\n", opts.Host)
	}

	var playbook string

	if !opts.Install {
		playbook = args[1]
	}

	t := &Tachyon{
		Target:      opts.Host,
		Debug:       opts.Debug,
		Output:      opts.ShowOutput,
		Clean:       opts.CleanHost,
		Dev:         opts.Development,
		Playbook:    playbook,
		Release:     opts.Release,
		InstallOnly: opts.Install,
	}

	_, err := RunAdhocCommand(t, t.Debug, t.Output, "")
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return 1
	}

	return 0
}
*/
