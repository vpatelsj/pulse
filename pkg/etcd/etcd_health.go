package etcd

import (
	"context"
	"log"
	"os"
	"regexp"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/pkg/transport"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	EtcdV3CACertFile = "/etc/kubernetes/pki/etcd/ca.crt"
	EtcdV3CertFile   = "/etc/kubernetes/pki/etcd/server.crt"
	EtcdV3KeyFile    = "/etc/kubernetes/pki/etcd/server.key"
)

type EtcdHealthCheck struct {
	CACertFile  string
	CertFile    string
	CertKeyFile string
	Endpoints   []string
	Logger      *logrus.Logger
}

type NodeInfo struct {
	name   string
	etcdIP string
	kubeIP string
}

func (e *EtcdHealthCheck) RunE(cmd *cobra.Command, _ []string) error {
	e.CACertFile = EtcdV3CACertFile
	e.CertFile = EtcdV3CertFile
	e.CertKeyFile = EtcdV3KeyFile

	logger, err := newLogger()
	if err != nil {
		return err
	}

	tlsParams := &transport.TLSInfo{
		CAFile:   e.CACertFile,
		CertFile: e.CertFile,
		KeyFile:  e.CertKeyFile,
	}

	tlsConfig, err := tlsParams.ClientConfig()
	if err != nil {
		return err
	}

	logger.Info("Getting Etcd Cluster Info")
	cli, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"https://127.0.0.1:2379"},
		TLS:       tlsConfig,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer cli.Close()

	listResp, err := cli.MemberList(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	//logger.Info("members:", listResp.Members)
	nodes := []NodeInfo{}
	for _, memb := range listResp.Members {
		for _, u := range memb.PeerURLs {
			n := memb.Name
			re := regexp.MustCompile("([0-9]{1,3}[.]){3}[0-9]{1,3}")
			match := re.FindStringSubmatch(u)
			nodes = append(nodes, NodeInfo{
				name:   n,
				etcdIP: match[0],
				kubeIP: "",
			})
		}
	}

	for _, v := range nodes {
		logger.Infof("HostName: %s , EtcdIP: %s, KubeIP: %s", v.name, v.etcdIP, v.kubeIP)
	}
	return nil
}

func newLogger() (*logrus.Logger, error) {
	res := logrus.New()
	res.Out = os.Stdout

	parsedLevel, err := logrus.ParseLevel("Trace")
	if err != nil {
		return nil, err
	}

	res.SetLevel(parsedLevel)
	res.SetFormatter(&logrus.TextFormatter{})

	return res, nil
}
