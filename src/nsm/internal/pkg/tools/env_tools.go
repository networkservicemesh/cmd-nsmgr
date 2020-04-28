package tools

import (
	"fmt"
	"github.com/sirupsen/logrus"
)

func RetrieveGoEnv(cgoEnabled bool, goos, goarch string) (env []string, cgoEnv []string) {
	logrus.Infof("Process env variables CGO_ENABLED=%v GOOS=%v GOARCH=%v", cgoEnabled, goos, goarch)
	if !cgoEnabled {
		cgoEnv = append(cgoEnv, "CGO_ENABLED=0")
		env = cgoEnv
	}
	env = append(env, fmt.Sprintf("GOOS=%s", goos), fmt.Sprintf("GOARCH=%s", goarch))
	return
}
