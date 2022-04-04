/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"io"

	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/alpha"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/upgrade"
	kubeadmutil "k8s.io/kubernetes/cmd/kubeadm/app/util"
	// 注册kubeadm配置类型，因为CLI标志的生成取决于生成的默认值。

	"github.com/lithammer/dedent"
	"github.com/spf13/cobra"
)

// NewKubeadmCommand 返回 cobra.Command 去运行 kubeadm 命令
// out参数是标准输出
func NewKubeadmCommand(in io.Reader, out, err io.Writer) *cobra.Command {
	var rootfsPath string

	// 显示与kubeadm主命令相关的帮助信息
	cmds := &cobra.Command{
		Use:   "kubeadm",
		Short: "kubeadm: 轻松的启动一个安全的Kubernetes集群",
		Long: dedent.Dedent(`

			    ┌──────────────────────────────────────────────────────────┐
			    │ KUBEADM                                                  │
			    │ 轻松的启动一个安全的Kubernetes集群                       │
			    │                                                          │
			    │ 请在这里给我们反馈:                                      │
			    │ https://github.com/kubernetes/kubeadm/issues             │
			    └──────────────────────────────────────────────────────────┘

			使用示例:
				创建具有两个节点的Kubernetes集群，包括一个控制平面节点（用于
				控制这个集群）和一个工作节点（可以工作在任何节点的工作负载，
				像Pod或者Deployment）。

			    ┌──────────────────────────────────────────────────────────┐
			    │ 在第一个机器节点上执行:                                  │
			    ├──────────────────────────────────────────────────────────┤
			    │ control-plane# kubeadm init                              │
			    └──────────────────────────────────────────────────────────┘

			    ┌──────────────────────────────────────────────────────────┐
			    │ 在第二个机器节点上执行:                                  │
			    ├──────────────────────────────────────────────────────────┤
			    │ worker# kubeadm join <执行kubeadm init之后返回的参数>    │
			    └──────────────────────────────────────────────────────────┘

				你可以在更多的机器节点上重复第二步

		`),
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if rootfsPath != "" {
				if err := kubeadmutil.Chroot(rootfsPath); err != nil {
					return err
				}
			}
			return nil
		},
	}
	// end:显示与kubeadm主命令相关的帮助信息

	cmds.ResetFlags()

	// 挂载子命令
	cmds.AddCommand(newCmdCertsUtility(out))
	cmds.AddCommand(newCmdCompletion(out, ""))
	cmds.AddCommand(newCmdConfig(out))
	cmds.AddCommand(newCmdInit(out, nil))
	cmds.AddCommand(newCmdJoin(out, nil))
	cmds.AddCommand(newCmdReset(in, out, nil))
	cmds.AddCommand(newCmdVersion(out))
	cmds.AddCommand(newCmdToken(out, err))
	cmds.AddCommand(upgrade.NewCmdUpgrade(out))
	cmds.AddCommand(alpha.NewCmdAlpha())
	options.AddKubeadmOtherFlags(cmds.PersistentFlags(), &rootfsPath)
	cmds.AddCommand(newCmdKubeConfigUtility(out))
	// end:挂载子命令

	return cmds
}
