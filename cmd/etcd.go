package cmd

import (
	"github.com/spf13/cobra"
	"github.com/vpatelsj/pulse/pkg/etcd"
)

var cmdHandler = &etcd.EtcdHealthCheck{}
var etcdCmd = &cobra.Command{
	Use:   "checkEtcd",
	Short: "Check etcd cluster's Health",
	Long:  "Uses etcd client to inspect cluster-info output",
	RunE:  cmdHandler.RunE,
}

func init() {
	rootCmd.AddCommand(etcdCmd)
}
