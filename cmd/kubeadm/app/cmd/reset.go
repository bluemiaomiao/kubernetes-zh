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
	"fmt"
	"io"

	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmapiv1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta3"
	"k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/validation"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	phases "k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/reset"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	cmdutil "k8s.io/kubernetes/cmd/kubeadm/app/cmd/util"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	configutil "k8s.io/kubernetes/cmd/kubeadm/app/util/config"
	utilruntime "k8s.io/kubernetes/cmd/kubeadm/app/util/runtime"

	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/lithammer/dedent"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

var (
	iptablesCleanupInstructions = dedent.Dedent(`
		重置过程不会重置或清除 iptables 规则或 IPVS 表。
		如果您想要重设 iptables，您必须使用 iptables 命令手动重设。

		如果你的集群是使用 IPVA 设置的，那么运行 ipvsadm --clear 重置你的系统 IPVS 表。

		重置过程不会清除 kubeconfig 文件，您必须手动删除它们。
		请检查 $HOME/.kube/config 文件的内容。
	`)

	cniCleanupInstructions = dedent.Dedent(`
		重置进程不会清除 CNI 配置。 为此，您必须删除/etc/cni/net.d
	`)
)

// resetOptions defines all the options exposed via flags by kubeadm reset.
type resetOptions struct {
	certificatesDir       string
	criSocketPath         string
	forceReset            bool
	ignorePreflightErrors []string
	kubeconfigPath        string
}

// resetData defines all the runtime information used when running the kubeadm reset workflow;
// this data is shared across all the phases that are included in the workflow.
type resetData struct {
	certificatesDir       string
	client                clientset.Interface
	criSocketPath         string
	forceReset            bool
	ignorePreflightErrors sets.String
	inputReader           io.Reader
	outputWriter          io.Writer
	cfg                   *kubeadmapi.InitConfiguration
	dirsToClean           []string
}

// newResetOptions returns a struct ready for being used for creating cmd join flags.
func newResetOptions() *resetOptions {
	return &resetOptions{
		certificatesDir: kubeadmapiv1.DefaultCertificatesDir,
		forceReset:      false,
		kubeconfigPath:  kubeadmconstants.GetAdminKubeConfigPath(),
	}
}

// newResetData returns a new resetData struct to be used for the execution of the kubeadm reset workflow.
func newResetData(cmd *cobra.Command, options *resetOptions, in io.Reader, out io.Writer) (*resetData, error) {
	var cfg *kubeadmapi.InitConfiguration

	client, err := getClientset(options.kubeconfigPath, false)
	if err == nil {
		klog.V(1).Infof("[reset] Loaded client set from kubeconfig file: %s", options.kubeconfigPath)
		cfg, err = configutil.FetchInitConfigurationFromCluster(client, out, "reset", false, false)
		if err != nil {
			klog.Warningf("[reset] Unable to fetch the kubeadm-config ConfigMap from cluster: %v", err)
		}
	} else {
		klog.V(1).Infof("[reset] Could not obtain a client set from the kubeconfig file: %s", options.kubeconfigPath)
	}

	ignorePreflightErrorsSet, err := validation.ValidateIgnorePreflightErrors(options.ignorePreflightErrors, ignorePreflightErrors(cfg))
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		// Also set the union of pre-flight errors to InitConfiguration, to provide a consistent view of the runtime configuration:
		cfg.NodeRegistration.IgnorePreflightErrors = ignorePreflightErrorsSet.List()
	}

	var criSocketPath string
	if options.criSocketPath == "" {
		criSocketPath, err = resetDetectCRISocket(cfg)
		if err != nil {
			return nil, err
		}
		klog.V(1).Infof("[reset] Detected and using CRI socket: %s", criSocketPath)
	} else {
		criSocketPath = options.criSocketPath
		klog.V(1).Infof("[reset] Using specified CRI socket: %s", criSocketPath)
	}

	return &resetData{
		certificatesDir:       options.certificatesDir,
		client:                client,
		criSocketPath:         criSocketPath,
		forceReset:            options.forceReset,
		ignorePreflightErrors: ignorePreflightErrorsSet,
		inputReader:           in,
		outputWriter:          out,
		cfg:                   cfg,
	}, nil
}

func ignorePreflightErrors(cfg *kubeadmapi.InitConfiguration) []string {
	if cfg == nil {
		return []string{}
	}
	return cfg.NodeRegistration.IgnorePreflightErrors
}

// AddResetFlags adds reset flags
func AddResetFlags(flagSet *flag.FlagSet, resetOptions *resetOptions) {
	flagSet.StringVar(
		&resetOptions.certificatesDir, options.CertificatesDir, resetOptions.certificatesDir,
		`The path to the directory where the certificates are stored. If specified, clean this directory.`,
	)
	flagSet.BoolVarP(
		&resetOptions.forceReset, options.ForceReset, "f", false,
		"Reset the node without prompting for confirmation.",
	)

	options.AddKubeConfigFlag(flagSet, &resetOptions.kubeconfigPath)
	options.AddIgnorePreflightErrorsFlag(flagSet, &resetOptions.ignorePreflightErrors)
	cmdutil.AddCRISocketFlag(flagSet, &resetOptions.criSocketPath)
}

// newCmdReset 返回kubeadm reset命令
func newCmdReset(in io.Reader, out io.Writer, resetOptions *resetOptions) *cobra.Command {
	if resetOptions == nil {
		resetOptions = newResetOptions()
	}
	resetRunner := workflow.NewRunner()

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "尽力还原“kubeadm init”或“kubeadm join”对此主机所做的更改",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resetRunner.InitData(args)
			if err != nil {
				return err
			}

			err = resetRunner.Run(args)
			if err != nil {
				return err
			}

			// 从有状态的kubelet、etcd和cni目录中清除内容
			data := c.(*resetData)
			cleanDirs(data)

			// 显示帮助文本指导用户如何删除cni目录
			fmt.Print(cniCleanupInstructions)
			// 显示帮助文本知道用户如何删除iptables规则
			fmt.Print(iptablesCleanupInstructions)
			return nil
		},
	}

	AddResetFlags(cmd.Flags(), resetOptions)

	// 将 Phase 添加到 Runner 中
	resetRunner.AppendPhase(phases.NewPreflightPhase())
	resetRunner.AppendPhase(phases.NewUpdateClusterStatus())
	resetRunner.AppendPhase(phases.NewRemoveETCDMemberPhase())
	resetRunner.AppendPhase(phases.NewCleanupNodePhase())

	// 设置数据构建器功能，运行整个 Workflow 或单个 Phase 时，Runner 都将使用该功能
	resetRunner.SetDataInitializer(func(cmd *cobra.Command, args []string) (workflow.RunData, error) {
		return newResetData(cmd, resetOptions, in, out)
	})

	// 通过修改命令帮助,添加 --skip-phases 标志和添加 Phase 子命令，将 Runner 绑定到 kubeadm init 命令
	resetRunner.BindToCommand(cmd)

	return cmd
}

func cleanDirs(data *resetData) {
	fmt.Printf("[reset] Deleting contents of stateful directories: %v\n", data.dirsToClean)
	for _, dir := range data.dirsToClean {
		klog.V(1).Infof("[reset] Deleting contents of %s", dir)
		if err := phases.CleanDir(dir); err != nil {
			klog.Warningf("[reset] Failed to delete contents of %q directory: %v", dir, err)
		}
	}
}

// Cfg returns the InitConfiguration.
func (r *resetData) Cfg() *kubeadmapi.InitConfiguration {
	return r.cfg
}

// CertificatesDir returns the CertificatesDir.
func (r *resetData) CertificatesDir() string {
	return r.certificatesDir
}

// Client returns the Client for accessing the cluster.
func (r *resetData) Client() clientset.Interface {
	return r.client
}

// ForceReset returns the forceReset flag.
func (r *resetData) ForceReset() bool {
	return r.forceReset
}

// InputReader returns the io.reader used to read messages.
func (r *resetData) InputReader() io.Reader {
	return r.inputReader
}

// IgnorePreflightErrors returns the list of preflight errors to ignore.
func (r *resetData) IgnorePreflightErrors() sets.String {
	return r.ignorePreflightErrors
}

// AddDirsToClean add a list of dirs to the list of dirs that will be removed.
func (r *resetData) AddDirsToClean(dirs ...string) {
	r.dirsToClean = append(r.dirsToClean, dirs...)
}

// CRISocketPath returns the criSocketPath.
func (r *resetData) CRISocketPath() string {
	return r.criSocketPath
}

func resetDetectCRISocket(cfg *kubeadmapi.InitConfiguration) (string, error) {
	if cfg != nil {
		// first try to get the CRI socket from the cluster configuration
		return cfg.NodeRegistration.CRISocket, nil
	}

	// if this fails, try to detect it
	return utilruntime.DetectCRISocket()
}
