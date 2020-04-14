package deviceapi

import (
	"context"
	"github.com/networkservicemesh/sdk/pkg/tools/serialize"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	"net"
	"os"
	"path"
)

type EventType = uint8

const (
	EventRegister EventType = iota
	EventUpdate
)

type Event struct {
	Kind     EventType
	Endpoint string
	Devices  []*pluginapi.Device
}
type Server interface {
	Stop()
	Start() error
	Allocate(ctx context.Context, deviceIds ...string) (resp *pluginapi.AllocateResponse, err error)
	GetDevices() (devices []string)
	GetClientEvents() chan *Event
	GetRegistrySocket() string
}

type clientEntry struct {
	request      *pluginapi.RegisterRequest
	conn         *grpc.ClientConn
	clientCtx    context.Context
	clientCancel context.CancelFunc
	devices      []*pluginapi.Device
	deviceApi    pluginapi.DevicePluginClient
}
type serverImpl struct {
	socketFile string
	server     *grpc.Server
	clients    map[string]*clientEntry
	executor   serialize.Executor
	listener   net.Listener
	events     chan *Event
	baseDir    string
}

func (s *serverImpl) GetRegistrySocket() string {
	return s.socketFile
}
func (s *serverImpl) GetClientEvents() chan *Event {
	return s.events
}

func (s *serverImpl) Register(ctx context.Context, request *pluginapi.RegisterRequest) (*pluginapi.Empty, error) {
	var err error
	<-s.executor.AsyncExec(func() {
		clientCtx, clientCancel := context.WithCancel(context.Background())
		entry := &clientEntry{
			request:      request,
			clientCtx:    clientCtx,
			clientCancel: clientCancel,
		}
		s.clients[request.Endpoint] = entry

		// Connect back
		entry.conn, err = grpc.DialContext(context.Background(), "unix:"+path.Join(s.baseDir, request.Endpoint), grpc.WithBlock(), grpc.WithInsecure())
		entry.deviceApi = pluginapi.NewDevicePluginClient(entry.conn)

		s.events <- &Event{
			Kind:     EventRegister,
			Endpoint: entry.request.Endpoint,
		}

		// Start List and Watch goroutine.
		go func() {
			var listApi pluginapi.DevicePlugin_ListAndWatchClient
			listApi, err = entry.deviceApi.ListAndWatch(entry.clientCtx, &pluginapi.Empty{})

			for {
				evt, err := listApi.Recv()
				if err != nil {
					logrus.Errorf("err receive event %v", err)
					return
				}
				entry.devices = evt.Devices
				s.events <- &Event{
					Kind:     EventUpdate,
					Endpoint: entry.request.Endpoint,
					Devices:  evt.Devices,
				}
			}
		}()

	})

	return &pluginapi.Empty{}, err
}

//NewServer - created a mock kubelet server to perform testing.
func NewServer(baseDir, socketFile string) Server {
	return &serverImpl{
		baseDir:    baseDir,
		socketFile: path.Join(baseDir, socketFile),
		clients:    map[string]*clientEntry{},
		events:     make(chan *Event),
	}
}

func (s *serverImpl) Start() error {
	s.server = grpc.NewServer()
	pluginapi.RegisterRegistrationServer(s.server, s)

	_ = os.Remove(s.socketFile)
	var err error
	s.listener, err = net.Listen("unix", s.socketFile)
	if err != nil {
		return err
	}
	go func() {
		_ = s.server.Serve(s.listener)
	}()
	return nil
}

func (s *serverImpl) Stop() {
	s.server.Stop()
	_ = s.listener.Close()
}

func (s *serverImpl) GetDevices() (devices []string) {
	<-s.executor.AsyncExec(func() {
		for _, c := range s.clients {
			for _, d := range c.devices {
				devices = append(devices, d.ID)
			}
		}
	})
	return
}

func (s *serverImpl) Allocate(ctx context.Context, deviceIds ...string) (resp *pluginapi.AllocateResponse, err error) {
	<-s.executor.AsyncExec(func() {
		for _, k := range s.clients {
			resp, err = k.deviceApi.Allocate(ctx, &pluginapi.AllocateRequest{
				ContainerRequests: []*pluginapi.ContainerAllocateRequest{
					{
						DevicesIDs: deviceIds,
					},
				},
			})
			if err == nil {
				return
			}
		}
	})
	return
}
