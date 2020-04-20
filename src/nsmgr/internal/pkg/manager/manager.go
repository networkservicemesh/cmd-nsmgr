package manager

import (
	"context"
	"net"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/constants"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/deviceplugin"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/flags"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/nsmgr"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcoptions"
	"github.com/networkservicemesh/sdk/pkg/tools/spanhelper"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/spiffe"
	"google.golang.org/grpc"
)

func RunNsmgr(ctx context.Context, values *flags.DefinedFlags) error {
	span := spanhelper.FromContext(ctx, "run")
	defer span.Finish()
	ctx = span.Context()

	var err error
	// Capture signals to cleanup before exiting - note: this *must* be the first thing in main
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs,
		os.Interrupt,
		// More Linux signals here
		syscall.SIGHUP,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	// Setup logging
	logrus.SetReportCaller(true)

	// Context to use for all things started in main
	var cancelFunc context.CancelFunc
	ctx, cancelFunc = context.WithCancel(ctx)
	defer cancelFunc()

	var spiffieTLSPeer *spiffe.TLSPeer
	spiffieTLSPeer, err = spiffe.NewTLSPeer(spiffe.WithWorkloadAPIAddr(values.SpiffeAgentURL.String()))
	if err != nil {
		span.LogErrorf("failed to create new spiffe TLS Peer %v", err)
		return err
	}

	regSpan := spanhelper.FromContext(ctx, "dial-registry")
	defer regSpan.Finish()
	var registryCC grpc.ClientConnInterface
	registryCC, err = grpc.DialContext(regSpan.Context(), values.RegistryURL.String(), grpcoptions.WithSpiffe(spiffieTLSPeer, 15*time.Second), grpc.WithBlock())

	if err != nil {
		regSpan.LogErrorf("failed to dial NSE Registry", err)
		return err
	}
	regSpan.Finish()

	mgr := nsmgr.NewServer(values.Name, nil, registryCC)

	nsmDir := path.Join(values.BaseDir, "nsm")
	_ = os.MkdirAll(nsmDir, os.ModeDir|os.ModePerm)
	var listener net.Listener
	listener, err = net.Listen("unix", path.Join(nsmDir, constants.NsmServerSocket))
	if err != nil {
		// Note: There's nothing productive we can do about this other than failing here
		// and thus not increasing the device pool
		return err
	}

	grpcServer := grpc.NewServer(grpcoptions.SpiffeCreds(spiffieTLSPeer, 15*time.Second))
	mgr.Register(grpcServer)

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.Stop()

	// Start device plugin
	dp := deviceplugin.NewServer(values)

	var watcher *fsnotify.Watcher
	watcher, err = createWatcher(values)
	defer func() {
		if watcher != nil {
			_ = watcher.Close()
		}
	}()
restart:
	if ctx.Err() != nil {
		return nil
	}
	dp.Stop()
	err = dp.Start()
	if err != nil {
		goto restart
	}
events:
	for {
		select {
		case <-ctx.Done():
			logrus.Infof("Command was canceled")
			return nil
		case event := <-watcher.Events:
			if event.Name == values.DeviceAPIRegistryServer && (event.Op&fsnotify.Create == fsnotify.Create) {
				logrus.Printf("inotify: %s created, restarting.", values.DeviceAPIRegistryServer)
				goto restart
			}

		case ierr := <-watcher.Errors:
			logrus.Printf("inotify: %s", ierr)
		case s := <-sigs:
			switch s {
			case syscall.SIGHUP:
				logrus.Println("Received SIGHUP, restarting.")
				goto restart
			default:
				logrus.Printf("Received signal \"%v\", shutting down.", s)
				dp.Stop()
				break events
			}
		}
	}
	return nil
}

func createWatcher(values *flags.DefinedFlags) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logrus.Errorf("failed to create FS watcher %v", err)
		return nil, err
	}

	// Listen for kubelet device api register socket, we need to re-register in case this socket is deleted, created against.
	err = watcher.Add(values.DeviceAPIPluginPath)
	if err != nil {
		_ = watcher.Close()
		logrus.Errorf("failed to create FS watcher %v", err)
		return nil, err
	}
	return watcher, nil
}
