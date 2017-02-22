package main

import (
	"fmt"
	"golang.org/x/net/context"
	"path"
	"time"

	etcd "github.com/coreos/etcd/clientv3"
	"github.com/pachyderm/pachyderm/src/client"
	"github.com/pachyderm/pachyderm/src/client/pkg/grpcutil"
	"github.com/pachyderm/pachyderm/src/client/version"
	"github.com/pachyderm/pachyderm/src/server/pkg/cmdutil"
	wshim "github.com/pachyderm/pachyderm/src/server/pkg/worker"
	"google.golang.org/grpc"
)

type AppEnv struct {
	Port        uint16 `env:"PORT,default=650"`
	EtcdAddress string `env:"ETCD_PORT_2379_TCP_ADDR,required"`
	PpsPrefix   string `env:"PPS_PREFIX,required"`
	PpsWorkerIp string `env:"PPS_WORKER_IP,required"`
}

func main() {
	cmdutil.Main(do, &AppEnv{})
}

func do(appEnvObj interface{}) error {
	appEnv := appEnvObj.(*AppEnv)
	etcdClient, _ := etcd.New(etcd.Config{
		Endpoints:   []string{fmt.Sprintf("%s:2379", appEnv.EtcdAddress)},
		DialTimeout: 15 * time.Second,
	})
	apiServer := wshim.ApiServer{
		EtcdClient: etcdClient,
	}
	err := grpcutil.Serve(
		func(s *grpc.Server) {
			wshim.RegisterWorkerServer(s, &apiServer)
		},
		grpcutil.ServeOptions{
			Version:    version.Version,
			MaxMsgSize: client.MaxMsgSize,
		},
		grpcutil.ServeEnv{
			GRPCPort: appEnv.Port,
		},
	)
	if err != nil {
		return err
	}
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	etcdClient.Put(ctx,
		path.Join(appEnv.PpsPrefix, "workers", appEnv.PpsWorkerIp), "")
	return nil
}