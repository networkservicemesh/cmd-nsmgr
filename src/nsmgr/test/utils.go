package test

import (
	"github.com/sirupsen/logrus"
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
	socketFile, _ := ioutil.TempDir(baseDir, "nsm_test")
	return socketFile
}
