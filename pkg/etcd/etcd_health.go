package etcd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"

	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/client-go/kubernetes"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/pkg/transport"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

type ClusterNodes struct {
	etcdNodes map[string]ETCDNode
	kubeNodes map[string]KubeNode
	vmssNodes map[string]VMSSNode
}

type ETCDNode struct {
	healthy bool
	ip      string
	name    string
}

type KubeNode struct {
	ready bool
	name  string
	ip    string
}

type VMSSNode struct {
	name        string
	ip          string
	latestModel bool
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
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"https://127.0.0.1:2379"},
		TLS:       tlsConfig,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer etcdClient.Close()

	clusterNodes := ClusterNodes{}

	clusterNodes.populateEtcdNodes(etcdClient)

	kc, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(kc)
	if err != nil {
		return err
	}

	err = clusterNodes.populateKubeNodes(kubeClient)
	if err != nil {
		return err
	}
	logger.Info(clusterNodes)
	if len(clusterNodes.kubeNodes) != len(clusterNodes.etcdNodes) {
		return errors.New("Etcd and Kube Nodes count does not match")
	}

	for name, etcdNode := range clusterNodes.etcdNodes {
		kubeNode, err := clusterNodes.kubeNodes[name]
		if !err || kubeNode.ip != etcdNode.ip {
			err1 := fmt.Sprintf("IP Mismatch for node %s", name)
			return errors.New(err1)
		}
	}

	for name, kubeNode := range clusterNodes.kubeNodes {
		etcdNode, err := clusterNodes.etcdNodes[name]
		if !err || kubeNode.ip != etcdNode.ip {
			err1 := fmt.Sprintf("IP Mismatch for node %s", name)
			return errors.New(err1)
		}
	}

	logger.Info("Etcd and Kube master node IP Matched")
	return nil
}

func GetNodeHostIP(node *v1.Node) (string, error) {
	addresses := node.Status.Addresses
	addressMap := make(map[v1.NodeAddressType][]v1.NodeAddress)
	for i := range addresses {
		addressMap[addresses[i].Type] = append(addressMap[addresses[i].Type], addresses[i])
	}
	if addresses, ok := addressMap[v1.NodeInternalIP]; ok {
		return addresses[0].Address, nil
	}
	return "", fmt.Errorf("host IP unknown; known addresses: %v", addresses)
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

func (c *ClusterNodes) populateEtcdNodes(cli *clientv3.Client) {
	listResp, err := cli.MemberList(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	nodes := make(map[string]ETCDNode, len(listResp.Members))
	for _, memb := range listResp.Members {
		for _, u := range memb.PeerURLs {
			n := memb.Name
			re := regexp.MustCompile("([0-9]{1,3}[.]){3}[0-9]{1,3}")
			match := re.FindStringSubmatch(u)
			nodes[n] = ETCDNode{
				name: n,
				ip:   match[0],
			}
		}
	}
	c.etcdNodes = nodes
}

func (c *ClusterNodes) populateKubeNodes(cli *kubernetes.Clientset) error {

	kubeNodes, err := cli.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	nodes := make(map[string]KubeNode, len(kubeNodes.Items))
	for _, n := range kubeNodes.Items {
		if strings.Contains(n.Name, "master") {
			nodeIp, err := GetNodeHostIP(&n)
			if err != nil {
				return err
			}
			nodes[n.Name] = KubeNode{
				name: n.Name,
				ip:   nodeIp,
			}
		}
	}
	c.kubeNodes = nodes
	return nil
}
