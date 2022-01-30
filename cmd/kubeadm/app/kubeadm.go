package app

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/kubernetes/cmd/kubeadm/app/cmd"

	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"

	"github.com/spf13/pflag"
)

// Run 创建和执行新的kubeadm命令
func Run() error {
	klog.InitFlags(nil)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Set("logtostderr", "true")
	// 我们不希望这些标志出现在--help中
	// 这些MarkHidden调用必须位于上述行之后
	pflag.CommandLine.MarkHidden("version")
	pflag.CommandLine.MarkHidden("log-flush-frequency")
	pflag.CommandLine.MarkHidden("alsologtostderr")
	pflag.CommandLine.MarkHidden("log-backtrace-at")
	pflag.CommandLine.MarkHidden("log-dir")
	pflag.CommandLine.MarkHidden("logtostderr")
	pflag.CommandLine.MarkHidden("stderrthreshold")
	pflag.CommandLine.MarkHidden("vmodule")

	fmt.Println("执行: cmd/kubeadm/app/kubeadm.go[Run][NewKubeadmCommand]")
	cmd := cmd.NewKubeadmCommand(os.Stdin, os.Stdout, os.Stderr)
	return cmd.Execute()
}
