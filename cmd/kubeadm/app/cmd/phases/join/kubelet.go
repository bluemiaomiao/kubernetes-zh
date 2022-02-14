/*
Copyright 2019 The Kubernetes Authors.

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

package phases

import (
	"context"
	"fmt"
	"os"

	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	kubeletphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/kubelet"
	patchnodephase "k8s.io/kubernetes/cmd/kubeadm/app/phases/patchnode"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/apiclient"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog/v2"

	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
)

var (
	kubeadmJoinFailMsg = dedent.Dedent(`
		Unfortunately, an error has occurred:
			%v

		This error is likely caused by:
			- The kubelet is not running
			- The kubelet is unhealthy due to a misconfiguration of the node in some way (required cgroups disabled)

		If you are on a systemd-powered system, you can try to troubleshoot the error with the following commands:
			- 'systemctl status kubelet'
			- 'journalctl -xeu kubelet'
		`)
)

// NewKubeletStartPhase 创建一个kubeadm工作流阶段，在节点上启动kubelet。
func NewKubeletStartPhase() workflow.Phase {
	return workflow.Phase{
		Name:  "kubelet-start [api-server-endpoint]",
		Short: "编写kubelet设置、证书并(重新)启动kubelet",
		Long:  "编写一个带有KubeletConfiguration的文件和一个带有节点特定kubelet设置的环境文件，然后(重新)启动kubelet。",
		Run:   runKubeletStartJoinPhase,
		InheritFlags: []string{
			options.CfgPath,
			options.NodeCRISocket,
			options.NodeName,
			options.FileDiscovery,
			options.TokenDiscovery,
			options.TokenDiscoveryCAHash,
			options.TokenDiscoverySkipCAHash,
			options.TLSBootstrapToken,
			options.TokenStr,
		},
	}
}

func getKubeletStartJoinData(c workflow.RunData) (*kubeadmapi.JoinConfiguration, *kubeadmapi.InitConfiguration, *clientcmdapi.Config, error) {
	data, ok := c.(JoinData)
	if !ok {
		return nil, nil, nil, errors.New("kubelet-start phase invoked with an invalid data struct")
	}
	cfg := data.Cfg()
	initCfg, err := data.InitCfg()
	if err != nil {
		return nil, nil, nil, err
	}
	tlsBootstrapCfg, err := data.TLSBootstrapCfg()
	if err != nil {
		return nil, nil, nil, err
	}
	return cfg, initCfg, tlsBootstrapCfg, nil
}

// runKubeletStartJoinPhase 执行kubelet TLS引导进程。
// 这个过程由kubelet执行，并在节点加入集群时按照节点授权者的要求使用一组专用凭证完成
func runKubeletStartJoinPhase(c workflow.RunData) (returnErr error) {
	cfg, initCfg, tlsBootstrapCfg, err := getKubeletStartJoinData(c)
	if err != nil {
		return err
	}
	bootstrapKubeConfigFile := kubeadmconstants.GetBootstrapKubeletKubeConfigPath()

	// 删除bootstrapKubeConfigFile，以便从磁盘中删除用于TLS引导的凭据
	defer os.Remove(bootstrapKubeConfigFile)

	// 将引导库文件或TLS引导库文件写入磁盘
	klog.V(1).Infof("[kubelet-start] writing bootstrap kubelet config file at %s", bootstrapKubeConfigFile)
	if err := kubeconfigutil.WriteToDisk(bootstrapKubeConfigFile, tlsBootstrapCfg); err != nil {
		return errors.Wrap(err, "couldn't save bootstrap-kubelet.conf to disk")
	}

	// 将ca证书写入磁盘，以便kubelet可以使用它进行身份验证
	cluster := tlsBootstrapCfg.Contexts[tlsBootstrapCfg.CurrentContext].Cluster
	if _, err := os.Stat(cfg.CACertPath); os.IsNotExist(err) {
		klog.V(1).Infof("[kubelet-start] writing CA certificate at %s", cfg.CACertPath)
		if err := certutil.WriteCert(cfg.CACertPath, tlsBootstrapCfg.Clusters[cluster].CertificateAuthorityData); err != nil {
			return errors.Wrap(err, "couldn't save the CA certificate to disk")
		}
	}

	bootstrapClient, err := kubeconfigutil.ClientSetFromFile(bootstrapKubeConfigFile)
	if err != nil {
		return errors.Errorf("couldn't create client from kubeconfig file %q", bootstrapKubeConfigFile)
	}

	// 获取此节点的名称。
	nodeName, _, err := kubeletphase.GetNodeNameAndHostname(&cfg.NodeRegistration)
	if err != nil {
		klog.Warning(err)
	}

	// 如果群集中存在同名节点，并且该节点处于“就绪”状态，请确保在TLS引导之前退出。
	// 与现有控制平面节点同名的新节点可能会导致未定义的行为，并最终导致控制平面故障。
	klog.V(1).Infof("[kubelet-start] 正在检查群集中名称为%q, 状态为%q的现有节点", nodeName, v1.NodeReady)
	node, err := bootstrapClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "cannot get Node %q", nodeName)
	}
	for _, cond := range node.Status.Conditions {
		if cond.Type == v1.NodeReady && cond.Status == v1.ConditionTrue {
			return errors.Errorf("群集中已经存在名为%q, 状态为%q的节点. "+
				"您必须删除现有节点或更改此新加入节点的名称", nodeName, v1.NodeReady)
		}
	}

	// 配置kubelet。在这么短的时间内，kubeadm试图停止/重启kubelet
	// 尝试停止kubelet服务，这样在配置时就不会出现竞争情况
	klog.V(1).Infoln("[kubelet-start] Stopping the kubelet")
	kubeletphase.TryStopKubelet()

	// 将kubelet的配置(使用引导令牌凭据)写入磁盘，以便kubelet可以启动
	if err := kubeletphase.WriteConfigToDisk(&initCfg.ClusterConfiguration, kubeadmconstants.KubeletRunDirectory); err != nil {
		return err
	}

	// 编写带有kubelet要使用的标志的env文件。如果节点不是控制平面，我们只想用指定的污点注册加入节点。否则，标记控制平面阶段将记录污点。
	registerTaintsUsingFlags := cfg.ControlPlane == nil
	if err := kubeletphase.WriteKubeletDynamicEnvFile(&initCfg.ClusterConfiguration, &initCfg.NodeRegistration, registerTaintsUsingFlags, kubeadmconstants.KubeletRunDirectory); err != nil {
		return err
	}

	// 尝试启动kubelet服务，以防它不活动
	fmt.Println("[kubelet-start] Starting the kubelet")
	kubeletphase.TryStartKubelet()

	// 现在kubelet将执行TLS引导，将/etc/kubernetes/Bootstrap-kubelet.conf转换为/etc/kubernetes/kubelet.conf
	// 等待kubelet创建/etc/kubernetes/kubelet.conf kubeconfig文件。如果此过程超时，显示一条用户友好的消息。
	waiter := apiclient.NewKubeWaiter(nil, kubeadmconstants.TLSBootstrapTimeout, os.Stdout)
	if err := waiter.WaitForKubeletAndFunc(waitForTLSBootstrappedClient); err != nil {
		fmt.Printf(kubeadmJoinFailMsg, err)
		return err
	}

	// 当我们知道/etc/kubernetes/kubelet.conf文件可用时，获取客户端
	client, err := kubeconfigutil.ClientSetFromFile(kubeadmconstants.GetKubeletKubeConfigPath())
	if err != nil {
		return err
	}

	klog.V(1).Infoln("[kubelet-start] preserving the crisocket information for the node")
	if err := patchnodephase.AnnotateCRISocket(client, cfg.NodeRegistration.Name, cfg.NodeRegistration.CRISocket); err != nil {
		return errors.Wrap(err, "error uploading crisocket")
	}

	return nil
}

// waitForTLSBootstrappedClient waits for the /etc/kubernetes/kubelet.conf file to be available
func waitForTLSBootstrappedClient() error {
	fmt.Println("[kubelet-start] Waiting for the kubelet to perform the TLS Bootstrap...")

	// Loop on every falsy return. Return with an error if raised. Exit successfully if true is returned.
	return wait.PollImmediate(kubeadmconstants.TLSBootstrapRetryInterval, kubeadmconstants.TLSBootstrapTimeout, func() (bool, error) {
		// Check that we can create a client set out of the kubelet kubeconfig. This ensures not
		// only that the kubeconfig file exists, but that other files required by it also exist (like
		// client certificate and key)
		_, err := kubeconfigutil.ClientSetFromFile(kubeadmconstants.GetKubeletKubeConfigPath())
		return (err == nil), nil
	})
}
