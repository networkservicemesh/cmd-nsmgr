package registry

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/uuid"
	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/sdk/pkg/tools/serialize"
	"google.golang.org/grpc"
)

type Server interface {
	registry.NsmRegistryServer
	registry.NetworkServiceRegistryServer

	Stop()
	Start(options ...grpc.ServerOption) error
	GetListenEndpointURI() url.URL
}

type serverImpl struct {
	socketFile string
	server     *grpc.Server
	executor   serialize.Executor
	listener   net.Listener
	endpoints  map[string]*registry.NSERegistration
	managers   map[string]*registry.NetworkServiceManager
}

func (s *serverImpl) GetListenEndpointURI() url.URL {
	result, _ := url.Parse("unix://" + s.socketFile)
	return *result
}

func (s *serverImpl) RegisterNSM(_ context.Context, manager *registry.NetworkServiceManager) (*registry.NetworkServiceManager, error) {
	s.executor.AsyncExec(func() {
		s.managers[manager.Url] = manager
	})
	return manager, nil
}

func (s *serverImpl) GetEndpoints(ctx context.Context, empty *empty.Empty) (*registry.NetworkServiceEndpointList, error) {
	panic("implement me")
}

func (s *serverImpl) RegisterNSE(ctx context.Context, registration *registry.NSERegistration) (*registry.NSERegistration, error) {
	s.executor.AsyncExec(func() {
		randomId, _ := uuid.NewRandom()
		registration.NetworkServiceEndpoint.Name = randomId.String()
		s.endpoints[randomId.String()] = registration
	})
	return registration, nil
}

func (s *serverImpl) BulkRegisterNSE(server registry.NetworkServiceRegistry_BulkRegisterNSEServer) error {
	return errors.New("Not implemented for mock")
}

func (s *serverImpl) RemoveNSE(ctx context.Context, request *registry.RemoveNSERequest) (*empty.Empty, error) {
	s.executor.AsyncExec(func() {
		delete(s.endpoints, request.NetworkServiceEndpointName)
	})
	return &empty.Empty{}, nil
}

//NewServer - created a mock kubelet server to perform testing.
func NewServer(socketFile string) Server {
	return &serverImpl{
		socketFile: socketFile,
		endpoints:  map[string]*registry.NSERegistration{},
	}
}

func (s *serverImpl) Start(options ...grpc.ServerOption) error {
	s.server = grpc.NewServer(options...)
	registry.RegisterNetworkServiceRegistryServer(s.server, s)
	registry.RegisterNsmRegistryServer(s.server, s)

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
