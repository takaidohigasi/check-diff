package main

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jessevdk/go-flags"
	"github.com/mackerelio/checkers"
)

// Version by Makefile
var Version string

type cmdOpts struct {
	OptArgs       []string
	OptCommand    string
	OptIdentifier string `long:"identifier" description:"indetify a file store the command result with given string"`
}

func runCmd(curFile *os.File, opts cmdOpts) error {
	cmd := exec.Command(opts.OptCommand, opts.OptArgs...)
	var stderr bytes.Buffer
	cmd.Stdout = curFile
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	err := cmd.Wait()
	if err != nil {
		return fmt.Errorf("%s - %s", err, stderr.String())
	}
	return nil
}

func runCopy(from string, to string) error {
	cmd := exec.Command("cp", from, to)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	err := cmd.Wait()
	if err != nil {
		return fmt.Errorf("%s - %s", err, stderr.String())
	}
	return nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func checkDiff(opts cmdOpts) *checkers.Checker {
	diffCmd, err := exec.LookPath("diff")
	if err != nil {
		return checkers.Critical(err.Error())
	}

	tmpDir := os.TempDir()

	hasher := md5.New()
	hasher.Write([]byte(opts.OptIdentifier))
	hasher.Write([]byte(opts.OptCommand))
	for _, v := range opts.OptArgs {
		hasher.Write([]byte(v))
	}
	commandKey := fmt.Sprintf("%x", hasher.Sum(nil))

	curUser, _ := user.Current()
	prevPath := filepath.Join(tmpDir, "check-diff-"+curUser.Uid+"-"+commandKey)

	curFile, err := ioutil.TempFile(tmpDir, "temp")
	if err != nil {
		return checkers.Critical(err.Error())
	}

	defer os.Remove(curFile.Name())

	err = runCmd(curFile, opts)
	if err != nil {
		return checkers.Critical(err.Error())
	}

	if !fileExists(prevPath) {
		err = runCopy(curFile.Name(), prevPath)
		if err != nil {
			return checkers.Critical(err.Error())
		}
		msg := ""
		if len(opts.OptArgs) > 0 {
			msg = fmt.Sprintf("first time execution command: '%s %s'", opts.OptCommand, strings.Join(opts.OptArgs, " "))
		} else {
			msg = fmt.Sprintf("first time execution command: '%s'", opts.OptCommand)
		}
		return checkers.Ok(msg)
	}

	// diff
	diffOut, diffError := exec.Command(diffCmd, "-U", "1", prevPath, curFile.Name()).Output()
	err = runCopy(curFile.Name(), prevPath)
	if err != nil {
		return checkers.Critical(err.Error())
	}

	if diffError == nil {
		// no difference
		curOpen, err := os.Open(curFile.Name())
		if err != nil {
			return checkers.Critical(err.Error())
		}
		defer curOpen.Close()

		fileinfo, _ := curOpen.Stat()
		data := make([]byte, 128)
		count, err := curOpen.Read(data)
		if err != nil {
			return checkers.Critical(err.Error())
		}
		cur := string(data[0:count])
		cur = regexp.MustCompile("(\r\n|\r|\n)$").ReplaceAllString(cur, "")
		msg := ""
		if fileinfo.Size() > 128 {
			msg = fmt.Sprintf("no difference: ```%s...```\n", cur)
		} else {
			msg = fmt.Sprintf("no difference: ```%s```\n", cur)
		}
		return checkers.Ok(msg)
	} else if regexp.MustCompile("exit status 1").MatchString(diffError.Error()) {
		// found diff
		diffRet := strings.Split(string(diffOut), "\n")
		diffRetString := strings.Join(diffRet[2:], "\n")
		diffRetString = regexp.MustCompile("(\r\n|\r|\n)$").ReplaceAllString(diffRetString, "")
		msg := ""
		if len(diffRetString) > 512 {
			msg = fmt.Sprintf("found difference: ```%s...```\n", diffRetString[0:512])
		} else {
			msg = fmt.Sprintf("found difference: ```%s```\n", diffRetString)
		}
		return checkers.Critical(msg)
	}

	return checkers.Critical(diffError.Error())
}

func main() {
	opts := cmdOpts{}
	psr := flags.NewParser(&opts, flags.Default)
	psr.Usage = "[OPTIONS] -- command args1 args2"
	args, err := psr.Parse()
	if err != nil {
		os.Exit(1)
	}
	if len(args) == 0 {
		psr.WriteHelp(os.Stdout)
		os.Exit(1)
	}

	opts.OptCommand = args[0]
	if len(args) > 1 {
		opts.OptArgs = args[1:]
	}

	ckr := checkDiff(opts)
	ckr.Name = "check-diff"
	ckr.Exit()
}
