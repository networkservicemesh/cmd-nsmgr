package test

import (
	"context"
	"github.com/docker/go-connections/nat"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/cmd"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/constants"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/flags"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/test/dockertest"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/test/mock/deviceapi"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/test/mock/registry"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcoptions"
	"github.com/spiffe/go-spiffe/spiffe"
	"github.com/stretchr/testify/require"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	"net/url"
	"os"
	"path"
	"sync"
	"testing"
	"time"
)

type testSetup struct {
	dt             dockertest.DockerTest
	spireContainer dockertest.DockerContainer
	baseDir        string
	mockDeviceApi  deviceapi.Server
	t              *testing.T
	registryServer registry.Server
	tlsPeer        *spiffe.TLSPeer
}

func (s *testSetup) SetupSpire() {
	s.spireContainer = s.dt.CreateContainer("test-spire", "networkservicemesh/test-spire-server", nil,
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
	s.spireContainer.Start()
	s.spireContainer.LogWaitPattern("Spire Proxy ready...", dockertest.DockerTimeout)

	flags.DefineFlags(func(f *flags.DefinedFlags) {
		f.Insecure = false
		agentUrl, _ := url.Parse("tcp://127.0.0.1:9099")
		f.SpiffeAgentURL = *agentUrl
	})
	var err error
	s.tlsPeer, err = spiffe.NewTLSPeer(spiffe.WithWorkloadAPIAddr(flags.Values.SpiffeAgentURL.String()))
	require.Nil(s.t, err)
}

func (s *testSetup) Start() {
	s.dt = dockertest.NewDockerTest(s.t)
	s.baseDir = TempFolder()
	flags.RestoreFlags()

	// Update flags
	flags.DefineFlags(func(f *flags.DefinedFlags) {
		f.BaseDir = s.baseDir
		f.Name = "nsm-test"
		f.Insecure = true
		f.DeviceAPIPluginPath = s.baseDir
	})
}

func (s *testSetup) Stop() {
	s.dt.Stop()
	_ = os.RemoveAll(s.baseDir)
	flags.RestoreFlags()

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
	flags.DefineFlags(func(f *flags.DefinedFlags) {
		f.DeviceAPIListenEndpoint = path.Join(s.baseDir, constants.KubeletServerSock)
		f.DeviceAPIPluginPath = s.baseDir
		f.DeviceAPIRegistryServer = s.mockDeviceApi.GetRegistrySocket()
	})
}

func (s *testSetup) StartRegistry() {
	s.registryServer = registry.NewServer(path.Join(s.baseDir, "registry.sock"))
	require.Nil(s.t, s.registryServer.Start(grpcoptions.SpiffeCreds(s.tlsPeer, 15*time.Second)))

	flags.DefineFlags(func(f *flags.DefinedFlags) {
		f.RegistryURL = s.registryServer.GetListenEndpointURI()
	})

}

func NewSetup(t *testing.T) *testSetup {
	setup := &testSetup{
		t: t,
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
		e := cmd.TestExecute(ctx, "run")
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
		e := cmd.TestExecute(ctx, "run")
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
