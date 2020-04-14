package main

import (
	"context"
	"google.golang.org/grpc/stats"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc/metadata"

	"github.com/sirupsen/logrus"
	proto "github.com/spiffe/go-spiffe/proto/spiffe/workload"
	"google.golang.org/grpc"
)

type spireProxy struct {
	workloadAPIClient proto.SpiffeWorkloadAPIClient
	cc                *grpc.ClientConn
}

func newSpireProxy() (*spireProxy, error) {
	return &spireProxy{
	}, nil
}

func (sp *spireProxy) Start(target string) error {
	var err error
	sp.cc, err = grpc.DialContext(context.Background(), target, grpc.WithBlock(), grpc.WithInsecure())
	if err != nil {
		logrus.Error(err)
		return err
	}
	sp.workloadAPIClient = proto.NewSpiffeWorkloadAPIClient(sp.cc)
	return nil
}

func (sp *spireProxy) Close() error {
	return sp.cc.Close()
}

func (sp *spireProxy) FetchJWTSVID(ctx context.Context, request *proto.JWTSVIDRequest) (*proto.JWTSVIDResponse, error) {
	logrus.Infof("FetchJWTSVID called...")

	header := metadata.Pairs("workload.spiffe.io", "true")
	grpcCtx := metadata.NewOutgoingContext(ctx, header)

	return sp.workloadAPIClient.FetchJWTSVID(grpcCtx, request)
}

func (sp *spireProxy) FetchJWTBundles(request *proto.JWTBundlesRequest, stream proto.SpiffeWorkloadAPI_FetchJWTBundlesServer) error {
	logrus.Infof("FetchJWTBundles called...")
	header := metadata.Pairs("workload.spiffe.io", "true")
	grpcCtx := metadata.NewOutgoingContext(stream.Context(), header)

	c, err := sp.workloadAPIClient.FetchJWTBundles(grpcCtx, request)
	if err != nil {
		logrus.Error(err)
		return err
	}

	for {
		if err := stream.Context().Err(); err != nil {
			logrus.Error(err)
			return err
		}

		msg, err := c.Recv()
		if err != nil {
			logrus.Error(err)
			return err
		}

		logrus.Infof("recv msg: %v", msg)

		if err := stream.Send(msg); err != nil {
			logrus.Error(err)
			return err
		}

		logrus.Infof("sent msg: %v", msg)
	}
}

func (sp *spireProxy) ValidateJWTSVID(ctx context.Context, request *proto.ValidateJWTSVIDRequest) (*proto.ValidateJWTSVIDResponse, error) {
	logrus.Infof("ValidateJWTSVID called...")
	header := metadata.Pairs("workload.spiffe.io", "true")
	grpcCtx := metadata.NewOutgoingContext(ctx, header)

	return sp.workloadAPIClient.ValidateJWTSVID(grpcCtx, request)
}

func (sp *spireProxy) FetchX509SVID(request *proto.X509SVIDRequest, stream proto.SpiffeWorkloadAPI_FetchX509SVIDServer) error {
	logrus.Info("FetchX509SVID called...")

	header := metadata.Pairs("workload.spiffe.io", "true")
	grpcCtx := metadata.NewOutgoingContext(stream.Context(), header)

	c, err := sp.workloadAPIClient.FetchX509SVID(grpcCtx, request)
	if err != nil {
		logrus.Error(err)
		return err
	}

	for {
		if err := stream.Context().Err(); err != nil {
			logrus.Error(err)
			return err
		}

		logrus.Infof("pre recv msg...")
		msg, err := c.Recv()
		if err != nil {
			logrus.Error(err)
			return err
		}

		logrus.Infof("recv msg: %v", msg)

		if err := stream.Send(msg); err != nil {
			logrus.Error(err)
			return err
		}

		logrus.Infof("sent msg: %v", msg)
	}
}

type grpcStatsHandler struct {
}

func (g grpcStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	logrus.Infof("TagRPC: %v", info)
	return ctx
}

func (g grpcStatsHandler) HandleRPC(ctx context.Context, stats stats.RPCStats) {
	logrus.Infof("HandleRPC: %v", stats)
}

func (g grpcStatsHandler) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	logrus.Infof("TagCon: %v", info)
	return ctx
}

func (g grpcStatsHandler) HandleConn(ctx context.Context, stats stats.ConnStats) {
	logrus.Infof("handleConn: %v", stats)
}

func main() {
	logrus.Infof("Spire Proxy started...")
	sigs := make(chan os.Signal, 1)
	grpc.EnableTracing = true
	signal.Notify(sigs,
		os.Interrupt,
		// More Linux signals here
		syscall.SIGHUP,
		syscall.SIGABRT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	srv := grpc.NewServer(grpc.StatsHandler(&grpcStatsHandler{}))

	proxy, err := newSpireProxy()
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() { _ = proxy.Close() }()

	proto.RegisterSpiffeWorkloadAPIServer(srv, proxy)

	var ln net.Listener
	ln, err = net.Listen("tcp", "0.0.0.0:9099")
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		if err := proxy.Start("unix:/tmp/agent.sock"); err != nil {
			logrus.Errorf("Failed to connect to spire")
		}
		logrus.Infof("Spire Proxy ready...")
		if err := srv.Serve(ln); err != nil {
			logrus.Fatal(err)
		}
	}()
	<-sigs
	logrus.Infof("Spire Proxy stopped...")
}
