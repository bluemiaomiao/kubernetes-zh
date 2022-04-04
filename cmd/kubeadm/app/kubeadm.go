package app

import (
	"flag"
	"os"

	"k8s.io/kubernetes/cmd/kubeadm/app/cmd"

	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"

	"github.com/spf13/pflag"
)

// Run 创建和执行新的kubeadm命令
func Run() error {
	// 用于实现 Google 内部 C++ 日志库一致的日志处理方式
	klog.InitFlags(nil)

	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	_ = pflag.Set("logtostderr", "true")
	// 我们不希望这些标志出现在--help中
	// 这些MarkHidden调用必须位于上述行之后
	_ = pflag.CommandLine.MarkHidden("version")
	_ = pflag.CommandLine.MarkHidden("log-flush-frequency")
	_ = pflag.CommandLine.MarkHidden("alsologtostderr")
	_ = pflag.CommandLine.MarkHidden("log-backtrace-at")
	_ = pflag.CommandLine.MarkHidden("log-dir")
	_ = pflag.CommandLine.MarkHidden("logtostderr")
	_ = pflag.CommandLine.MarkHidden("stderrthreshold")
	_ = pflag.CommandLine.MarkHidden("vmodule")

	// 创建 kubeadm 的 cobra.Command 结构体，并执行
	return cmd.NewKubeadmCommand(os.Stdin, os.Stdout, os.Stderr).Execute()
}
