package test

import (
	"context"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
)

func TempFolder() string {
	baseDir := path.Join(os.TempDir(), "nsm")
	err := os.MkdirAll(baseDir, os.ModeDir|os.ModePerm)
	if err != nil {
		logrus.Errorf("err: %v", err)
	}
	socketFile, _ := ioutil.TempDir(baseDir, "nsm_test")
	return socketFile
}

//ProcWrapper - A simple process wrapper
type ProcWrapper struct {
	Cmd    *exec.Cmd
	cancel context.CancelFunc
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

// ExitCode - wait for completion and return exit code
func (w *ProcWrapper) ExitCode() int {
	err := w.Cmd.Wait()
	if err != nil {
		e, ok := err.(*exec.ExitError)
		if ok {
			return e.ExitCode()
		}
		logrus.Errorf("Error during waiting for process exit code: %v %v", w.Cmd.Args, err)
		return -1
	}
	return w.Cmd.ProcessState.ExitCode()
}