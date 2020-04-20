package dockertest_test

import (
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/test/dockertest"
	"os"
	"testing"
)

func TestDockerAlive(t *testing.T) {
	if os.Getenv("NSM_FROM_DOCKER") == "true" {
		//Test should pass
		return
	}
	dt := dockertest.NewDockerTest(t)
	defer dt.Stop()
}
