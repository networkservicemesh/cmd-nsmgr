package test

import (
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"io/ioutil"
	"os"
	"path"
)

func TempFolder() string {
	baseDir := path.Join(os.TempDir(), "nsm")
	err := os.MkdirAll(baseDir, os.ModeDir|os.ModePerm)
	if err != nil {
		logrus.Errorf("err: %v", err)
	}
	socketFile, _ := ioutil.TempFile(baseDir, "nsm_test")
	_ = socketFile.Close()
	_ = os.Remove(socketFile.Name())
	unix.Umask(0077)
	_ = os.MkdirAll(socketFile.Name(), os.ModeDir|os.ModePerm)
	return socketFile.Name()
}
