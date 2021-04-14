package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/edwarnicke/exechelper"
	"github.com/golang/protobuf/ptypes/empty"
	registryclient "github.com/networkservicemesh/sdk/pkg/registry/chains/client"
	"github.com/networkservicemesh/sdk/pkg/registry/chains/memory"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/log/logruslogger"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/next"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/sandbox"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"
	"github.com/networkservicemesh/sdk/pkg/tools/spire"
	"github.com/networkservicemesh/sdk/pkg/tools/token"
)

func Test(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	var spireErrCh <-chan error
	var sutErrChs [2]<-chan error
	t.Cleanup(func() {
		cancel()
		if spireErrCh != nil {
			for range spireErrCh {
			}
		}
		for _, sutErrCh := range sutErrChs {
			if sutErrCh != nil {
				for range sutErrCh {
				}
			}
		}
	})

	logrus.SetFormatter(&nested.Formatter{})
	log.EnableTracing(true)
	ctx = log.Join(ctx, logruslogger.New(ctx))

	// --------------------------------------------------------------------------
	log.FromContext(ctx).Info("Start spire")
	// --------------------------------------------------------------------------
	executable, err := os.Executable()
	require.NoError(t, err)

	spireErrCh = spire.Start(
		spire.WithContext(ctx),
		spire.WithEntry("spiffe://example.org/nsmgr", "unix:path:/bin/nsmgr"),
		spire.WithEntry(fmt.Sprintf("spiffe://example.org/%s", filepath.Base(executable)),
			fmt.Sprintf("unix:path:%s", executable),
		),
	)
	require.Len(t, spireErrCh, 0)

	// --------------------------------------------------------------------------
	log.FromContext(ctx).Info("Get X509Source")
	// --------------------------------------------------------------------------
	source, err := workloadapi.NewX509Source(ctx)
	require.NoError(t, err)

	// --------------------------------------------------------------------------
	log.FromContext(ctx).Info("Start NSM")
	// --------------------------------------------------------------------------
	domain := sandbox.NewBuilder(ctx, t).
		UseUnixSockets().
		SetNodesCount(2).
		SetRegistrySupplier(memory.NewServer).
		SetNodeSetup(func(ctx context.Context, node *sandbox.Node, i int) {
			// --------------------------------------------------------------------------
			log.FromContext(ctx).Infof("Start NSMgr-%d", i)
			// --------------------------------------------------------------------------
			unixURL := &url.URL{Scheme: "unix", Path: "/nsm.io.sock"}

			cmdStr := "nsmgr"
			sutErrChs[i] = exechelper.Start(cmdStr,
				exechelper.WithContext(ctx),
				exechelper.WithEnvirons(os.Environ()...),
				exechelper.WithStdout(os.Stdout),
				exechelper.WithStderr(os.Stderr),
				exechelper.WithEnvKV("NSM_NAME", fmt.Sprint("nsmgr-", i)),
				exechelper.WithEnvKV("NSM_LISTEN_ON", fmt.Sprint(unixURL.String(), ",tcp://127.0.0.1:0")),
				exechelper.WithEnvKV("NSM_REGISTRY_URL", node.Registry.URL.String()),
			)
			require.Len(t, sutErrChs[i], 0)

			node.NSMgr = &sandbox.NSMgrEntry{
				URL: unixURL,
			}

			cc, err := grpc.DialContext(ctx, grpcutils.URLToTarget(unixURL),
				sandbox.DefaultSecureDialOptions(
					credentials.NewTLS(tlsconfig.MTLSClientConfig(source, source, tlsconfig.AuthorizeAny())),
					spiffejwt.TokenGeneratorFunc(source, sandbox.DefaultTokenTimeout),
				)...,
			)
			require.NoError(t, err)

			node.ForwarderRegistryClient = registryclient.NewNetworkServiceEndpointRegistryInterposeClient(ctx, cc)
			node.EndpointRegistryClient = registryclient.NewNetworkServiceEndpointRegistryClient(ctx, cc)
			node.NSRegistryClient = registryclient.NewNetworkServiceRegistryClient(cc)

			// --------------------------------------------------------------------------
			log.FromContext(ctx).Infof("Start Forwarder-%d", i)
			// --------------------------------------------------------------------------
			node.NewForwarder(ctx, &registry.NetworkServiceEndpoint{
				Name: fmt.Sprint("forwarder-", i),
			})
		}).
		SetServerTransportCredentialsSupplier(func() credentials.TransportCredentials {
			return credentials.NewTLS(tlsconfig.MTLSServerConfig(source, source, tlsconfig.AuthorizeAny()))
		}).
		SetClientTransportCredentialsSupplier(func() credentials.TransportCredentials {
			return credentials.NewTLS(tlsconfig.MTLSClientConfig(source, source, tlsconfig.AuthorizeAny()))
		}).
		SetTokenGeneratorSupplier(func(timeout time.Duration) token.GeneratorFunc {
			return spiffejwt.TokenGeneratorFunc(source, timeout)
		}).
		Build()

	// --------------------------------------------------------------------------
	log.FromContext(ctx).Info("Start Endpoint")
	// --------------------------------------------------------------------------
	nseReg := &registry.NetworkServiceEndpoint{
		Name:                "nse",
		NetworkServiceNames: []string{"ns"},
	}

	counter := new(counterServer)
	domain.Nodes[1].NewEndpoint(ctx, nseReg,
		sandbox.WithEndpointAdditionalFunctionality(counter),
	)

	// --------------------------------------------------------------------------
	log.FromContext(ctx).Info("Request with Client")
	// --------------------------------------------------------------------------
	request := &networkservice.NetworkServiceRequest{
		Connection: &networkservice.Connection{
			NetworkService: "ns",
		},
	}

	nsc := domain.Nodes[0].NewClient(ctx)

	requestCtx, requestCancel := context.WithTimeout(ctx, 15*time.Second)
	defer requestCancel()

	conn, err := nsc.Request(requestCtx, request.Clone())
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Equal(t, int32(1), counter.Requests)

	closeCtx, closeCancel := context.WithTimeout(ctx, 15*time.Second)
	defer closeCancel()

	_, err = nsc.Close(closeCtx, conn)
	require.NoError(t, err)
	require.Equal(t, int32(1), counter.Closes)
}

type counterServer struct {
	Requests, Closes int32
}

func (c *counterServer) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*networkservice.Connection, error) {
	atomic.AddInt32(&c.Requests, 1)

	return next.Server(ctx).Request(ctx, request)
}

func (c *counterServer) Close(ctx context.Context, connection *networkservice.Connection) (*empty.Empty, error) {
	atomic.AddInt32(&c.Closes, 1)

	return next.Server(ctx).Close(ctx, connection)
}
