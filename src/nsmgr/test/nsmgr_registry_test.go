package test

import (
	"context"
	"fmt"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/cmd"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffeutils"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"net/url"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/test/mock/registry"
	"github.com/spiffe/go-spiffe/spiffe"
	"github.com/stretchr/testify/require"
)

type testSetup struct {
	baseDir        string
	t              *testing.T
	registryServer registry.Server
	tlsPeer        *spiffe.TLSPeer
	values         cmd.DefinedFlags
}

func (s *testSetup) Start() {
	s.baseDir = TempFolder()

	// Update flags
	s.values.BaseDir = s.baseDir
	s.values.Name = "nsm-test"

	// Configure ListenOnURL
	url, _ := url.Parse(fmt.Sprintf("unix:///%s/%s", s.baseDir, cmd.NsmServerSocket))
	s.values.ListenOnURL = *url

	// Configure spire environment used
	url, _ = url.Parse("unix:///tmp/agent.sock")
	s.values.SpiffeAgentURL = *url

	var err error
	s.tlsPeer, err = spiffe.NewTLSPeer(spiffe.WithWorkloadAPIAddr(s.values.SpiffeAgentURL.String()))
	require.Nil(s.t, err)

	// Setup registry
	s.registryServer = registry.NewServer(path.Join(s.baseDir, "registry.sock"))
	require.Nil(s.t, s.registryServer.Start(spiffeutils.SpiffeCreds(s.tlsPeer, 15*time.Second)))

	s.values.RegistryURL = s.registryServer.GetListenEndpointURI()
}

func (s *testSetup) Stop() {
	_ = os.RemoveAll(s.baseDir)

	if s.registryServer != nil {
		s.registryServer.Stop()
	}
}

func NewSetup(t *testing.T) *testSetup {
	setup := &testSetup{
		t:      t,
		values: *cmd.Defaults,
	}
	setup.Start()
	return setup
}

func TestNSMgrRegister(t *testing.T) {
	setup := NewSetup(t)
	defer setup.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		e := cmd.RunNsmgr(ctx, &setup.values)
		require.Nil(t, e)
	}()

	// Check Health is ok
	checkHeal(t, setup)

	cancel()
	wg.Wait()
}

func checkHeal(t *testing.T, setup *testSetup) {
	tlsPeer, _ := spiffe.NewTLSPeer(spiffe.WithWorkloadAPIAddr(setup.values.SpiffeAgentURL.String()))
	clientCtx, clientCancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
	defer clientCancelFunc()
	healthCC, err := grpc.DialContext(clientCtx, setup.values.ListenOnURL.String(), grpc.WithBlock(), spiffeutils.WithSpiffe(tlsPeer, spiffeutils.DefaultTimeout))
	if err != nil {
		logrus.Fatalf("Failed healthcheck: %+v", err)
	}
	healthClient := grpc_health_v1.NewHealthClient(healthCC)
	healthResponse, err := healthClient.Check(clientCtx, &grpc_health_v1.HealthCheckRequest{
		Service: "networkservice.NetworkService",
	})
	assert.NoError(t, err)
	assert.NotNil(t, healthResponse)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, healthResponse.Status)
}

func TestNSMgrRegisterRestart(t *testing.T) {
	setup := NewSetup(t)
	defer setup.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = cmd.RunNsmgr(ctx, &setup.values)
	}()

	checkHeal(t, setup)

	cancel()
	wg.Wait()
}
