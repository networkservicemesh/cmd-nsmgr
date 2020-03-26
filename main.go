package main

import (
	"github.com/sirupsen/logrus"

	"github.com/networkservicemesh/cmd-nsmgr/pkg/cmd"
)

func main() {
	err := cmd.Execute()
	if err != nil {
		logrus.Fatalf("error executing rootCmd: %v", err)
	}
}
