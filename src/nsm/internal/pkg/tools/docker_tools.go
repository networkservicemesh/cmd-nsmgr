package tools

import (
	"io/ioutil"
	"os"
	"strings"
)

// IsDocker - tells we are running from inside docker or other container.
func IsDocker() bool {
	return isDockerFileExists() || isDockerHasCGroup()
}

func isDockerFileExists() bool {
	_, err := os.Stat("/.dockerenv")
	if err != nil {
		return false
	}
	return os.IsExist(err)
}

func isDockerHasCGroup() bool {
	content, err := ioutil.ReadFile("/proc/self/cgroup")
	if err != nil {
		return false
	}
	text := string(content)
	return strings.Contains(text, "docker") || strings.Contains(text, "lxc")
}
