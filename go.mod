module github.com/networkservicemesh/cmd-nsmgr

go 1.13

require (
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v1.13.1
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.4.0 // indirect
	github.com/fsnotify/fsnotify v1.4.7
	github.com/golang/protobuf v1.3.3
	github.com/google/uuid v1.1.1
	github.com/networkservicemesh/api v0.0.0-20200413003704-e57e43fc2237
	github.com/networkservicemesh/sdk v0.0.0-20200413004036-bebb7bdc2a05
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opentracing/opentracing-go v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.0-20181021141114-fe5e611709b0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.6.2
	github.com/spiffe/go-spiffe v0.0.0-20200115174642-4e401e3b85fe
	github.com/stretchr/testify v1.4.0
	golang.org/x/sys v0.0.0-20191022100944-742c48ecaeb7
	golang.org/x/text v0.3.2
	google.golang.org/grpc v1.28.0
	k8s.io/kubelet v0.18.0
)
