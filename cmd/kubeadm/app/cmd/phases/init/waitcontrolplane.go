/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"
	"io"
	"path/filepath"
	"text/template"
	"time"

	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/apiclient"
	dryrunutil "k8s.io/kubernetes/cmd/kubeadm/app/util/dryrun"

	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
)

var (
	kubeletFailTempl = template.Must(template.New("init").Parse(dedent.Dedent(`
	Unfortunately, an error has occurred:
		{{ .Error }}

	This error is likely caused by:
		- The kubelet is not running
		- The kubelet is unhealthy due to a misconfiguration of the node in some way (required cgroups disabled)

	If you are on a systemd-powered system, you can try to troubleshoot the error with the following commands:
		- 'systemctl status kubelet'
		- 'journalctl -xeu kubelet'

	Additionally, a control plane component may have crashed or exited when started by the container runtime.
	To troubleshoot, list all containers using your preferred container runtimes CLI.
{{ if .IsDocker }}
	Here is one example how you may list all Kubernetes containers running in docker:
		- 'docker ps -a | grep kube | grep -v pause'
		Once you have found the failing container, you can inspect its logs with:
		- 'docker logs CONTAINERID'
{{ else }}
	Here is one example how you may list all Kubernetes containers running in cri-o/containerd using crictl:
		- 'crictl --runtime-endpoint {{ .Socket }} ps -a | grep kube | grep -v pause'
		Once you have found the failing container, you can inspect its logs with:
		- 'crictl --runtime-endpoint {{ .Socket }} logs CONTAINERID'
{{ end }}
	`)))
)

// NewWaitControlPlanePhase 是在控制平面和etcd阶段之后运行的隐藏阶段
func NewWaitControlPlanePhase() workflow.Phase {
	phase := workflow.Phase{
		Name:   "wait-control-plane",
		Run:    runWaitControlPlanePhase,
		Hidden: true,
	}
	return phase
}

// 创建一个Waiter然后执行WaitForKubeletAndFunc, 从预定义的常量读取超时, 超时以后则API Server挂了
func runWaitControlPlanePhase(c workflow.RunData) error {
	data, ok := c.(InitData)
	if !ok {
		return errors.New("wait-control-plane阶段使用无效的数据结构")
	}

	// 如果我们在试运行，打印生成的清单
	if err := printFilesIfDryRunning(data); err != nil {
		return errors.Wrap(err, "在试运行上打印文件时出错")
	}

	// waiter holds the apiclient.Waiter implementation of choice, responsible for querying the API server in various ways and waiting for conditions to be fulfilled
	klog.V(1).Infoln("[wait-control-plane] 等待API Server正常运行")

	client, err := data.Client()
	if err != nil {
		return errors.Wrap(err, "无法获取客户端")
	}

	timeout := data.Cfg().ClusterConfiguration.APIServer.TimeoutForControlPlane.Duration
	waiter, err := newControlPlaneWaiter(data.DryRun(), timeout, client, data.OutputWriter())
	if err != nil {
		return errors.Wrap(err, "error creating waiter")
	}

	fmt.Printf("[wait-control-plane] 等待kubelet从目录中以静态Pods的形式启动控制平面 %q. 这可能需要 %v\n", data.ManifestDir(), timeout)

	if err := waiter.WaitForKubeletAndFunc(waiter.WaitForAPI); err != nil {
		context := struct {
			Error    string
			Socket   string
			IsDocker bool
		}{
			Error:    fmt.Sprintf("%v", err),
			Socket:   data.Cfg().NodeRegistration.CRISocket,
			IsDocker: data.Cfg().NodeRegistration.CRISocket == kubeadmconstants.DefaultDockerCRISocket,
		}

		// 使用template库渲染错误信息
		_ = kubeletFailTempl.Execute(data.OutputWriter(), context)
		return errors.New("无法初始化Kubernetes群集")
	}

	return nil
}

// printFilesIfDryRunning prints the Static Pod manifests to stdout and informs about the temporary directory to go and lookup
func printFilesIfDryRunning(data InitData) error {
	if !data.DryRun() {
		return nil
	}
	manifestDir := data.ManifestDir()

	fmt.Printf("[dryrun] Wrote certificates, kubeconfig files and control plane manifests to the %q directory\n", manifestDir)
	fmt.Println("[dryrun] The certificates or kubeconfig files would not be printed due to their sensitive nature")
	fmt.Printf("[dryrun] Please examine the %q directory for details about what would be written\n", manifestDir)

	// Print the contents of the upgraded manifests and pretend like they were in /etc/kubernetes/manifests
	files := []dryrunutil.FileToPrint{}
	// Print static pod manifests
	for _, component := range kubeadmconstants.ControlPlaneComponents {
		realPath := kubeadmconstants.GetStaticPodFilepath(component, manifestDir)
		outputPath := kubeadmconstants.GetStaticPodFilepath(component, kubeadmconstants.GetStaticPodDirectory())
		files = append(files, dryrunutil.NewFileToPrint(realPath, outputPath))
	}
	// Print kubelet config manifests
	kubeletConfigFiles := []string{kubeadmconstants.KubeletConfigurationFileName, kubeadmconstants.KubeletEnvFileName}
	for _, filename := range kubeletConfigFiles {
		realPath := filepath.Join(manifestDir, filename)
		outputPath := filepath.Join(kubeadmconstants.KubeletRunDirectory, filename)
		files = append(files, dryrunutil.NewFileToPrint(realPath, outputPath))
	}

	return dryrunutil.PrintDryRunFiles(files, data.OutputWriter())
}

// newControlPlaneWaiter 返回一个新的Waiter，用于等待控制平面启动。
func newControlPlaneWaiter(dryRun bool, timeout time.Duration, client clientset.Interface, out io.Writer) (apiclient.Waiter, error) {
	if dryRun {
		return dryrunutil.NewWaiter(), nil
	}

	return apiclient.NewKubeWaiter(client, timeout, out), nil
}
