package test

import (
	"context"
	"net/url"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/constants"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/flags"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/manager"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/test/dockertest"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/test/mock/deviceapi"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/test/mock/registry"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcoptions"
	"github.com/spiffe/go-spiffe/spiffe"
	"github.com/stretchr/testify/require"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type testSetup struct {
	dt             dockertest.DockerTest
	spireContainer dockertest.DockerContainer
	baseDir        string
	mockDeviceApi  deviceapi.Server
	t              *testing.T
	registryServer registry.Server
	tlsPeer        *spiffe.TLSPeer
	values         flags.DefinedFlags
}

func (s *testSetup) SetupSpire() {
	if s.dt != nil {
		// We need to build a container
		var err error
		s.spireContainer, err = s.dt.CreateContainer("test-spire", "nsmgr-test-container",
			[]string{"sh", "-c", "/bin/spire.sh && /bin/spire-proxy && tail -f /dev/null"},
			dockertest.ContainerConfig{
				Privileged: true,
				ExposedPorts: nat.PortSet{
					"9099/tcp": {},
				},
				PortBindings: nat.PortMap{
					"9099/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "9099",
						},
					},
				},
			})
		require.Nil(s.t, err, "Please build test container to access spire-server `docker build . --target test -t nsmgr-test-container`")
		err = s.spireContainer.Start()
		require.Nil(s.t, err, "Please build test container to access spire-server `docker build . --target test -t nsmgr-test-container`")
		s.spireContainer.LogWaitPattern("Spire Proxy ready...", dockertest.DockerTimeout)

		agentUrl, _ := url.Parse("tcp://127.0.0.1:9099")
		s.values.SpiffeAgentURL = *agentUrl
	} else {
		// We are inside docker env with tests, we just need to start spifie agent, and perform testing, no need for proxy.
		// Spire are up and running already at default place
		agentUrl, _ := url.Parse("unix:///tmp/agent.sock")
		s.values.SpiffeAgentURL = *agentUrl
	}

	var err error
	s.tlsPeer, err = spiffe.NewTLSPeer(spiffe.WithWorkloadAPIAddr(s.values.SpiffeAgentURL.String()))
	require.Nil(s.t, err)
}

func (s *testSetup) Start() {
	// if USE_DOCKER - is set,
	if os.Getenv("NSM_FROM_DOCKER") != "true" {
		// We are in dev environment
		s.dt = dockertest.NewDockerTest(s.t)
	}
	s.baseDir = TempFolder()

	// Update flags
	s.values.BaseDir = s.baseDir
	s.values.Name = "nsm-test"
	s.values.DeviceAPIPluginPath = s.baseDir
}

func (s *testSetup) Stop() {
	if s.dt != nil {
		s.dt.Stop()
	}
	_ = os.RemoveAll(s.baseDir)

	if s.registryServer != nil {
		s.registryServer.Stop()
	}

	if s.mockDeviceApi != nil {
		s.mockDeviceApi.Stop()
	}
}

func (s *testSetup) StartDeviceAPI() {
	s.mockDeviceApi = deviceapi.NewServer(path.Join(s.baseDir, "kubelet.sock"))
	require.Nil(s.t, s.mockDeviceApi.Start())

	// Update flags
	s.values.DeviceAPIListenEndpoint = path.Join(s.baseDir, constants.KubeletServerSock)
	s.values.DeviceAPIPluginPath = s.baseDir
	s.values.DeviceAPIRegistryServer = s.mockDeviceApi.GetRegistrySocket()
}

func (s *testSetup) StartRegistry() {
	s.registryServer = registry.NewServer(path.Join(s.baseDir, "registry.sock"))
	require.Nil(s.t, s.registryServer.Start(grpcoptions.SpiffeCreds(s.tlsPeer, 15*time.Second)))

	s.values.RegistryURL = s.registryServer.GetListenEndpointURI()
}

func NewSetup(t *testing.T) *testSetup {
	setup := &testSetup{
		t:      t,
		values: *flags.Defaults,
	}
	setup.Start()
	return setup
}

func TestNSMgrRegister(t *testing.T) {
	setup := NewSetup(t)

	defer setup.Stop()

	// Setup spire
	setup.SetupSpire()

	setup.StartDeviceAPI()
	setup.StartRegistry()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		e := manager.RunNsmgr(ctx, &setup.values)
		require.Nil(t, e)
	}()

	regEvent := <-setup.mockDeviceApi.GetClientEvents()
	require.Equal(t, constants.KubeletServerSock, regEvent.Endpoint)

	updateEvent := <-setup.mockDeviceApi.GetClientEvents()
	require.Equal(t, 30, len(updateEvent.Devices))

	// return key sorted list of devices
	devs := map[string]*pluginapi.Device{}
	for _, k := range updateEvent.Devices {
		devs[k.ID] = k
	}
	d, ok := devs["nsm-0"]
	require.True(t, ok)
	require.Equal(t, pluginapi.Healthy, d.Health)
	cancel()
	wg.Wait()
}

func TestNSMgrRegisterRestart(t *testing.T) {
	setup := NewSetup(t)

	defer setup.Stop()

	// Setup spire
	setup.SetupSpire()

	setup.StartDeviceAPI()
	setup.StartRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		e := manager.RunNsmgr(ctx, &setup.values)
		require.Nil(t, e)
	}()

	regEvent := <-setup.mockDeviceApi.GetClientEvents()
	require.Equal(t, constants.KubeletServerSock, regEvent.Endpoint)

	updateEvent := <-setup.mockDeviceApi.GetClientEvents()
	require.Equal(t, deviceapi.EventUpdate, updateEvent.Kind)
	require.Equal(t, 30, len(updateEvent.Devices))

	// We need to restart mock kubelet and check if server is registered.

	setup.mockDeviceApi.Stop()
	require.Nil(t, setup.mockDeviceApi.Start())

	// Check we have receive register again
	regEvent = <-setup.mockDeviceApi.GetClientEvents()
	require.Equal(t, constants.KubeletServerSock, regEvent.Endpoint)
	cancel()
	wg.Wait()
}
