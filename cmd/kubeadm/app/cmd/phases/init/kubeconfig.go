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
	"os"
	"path/filepath"

	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	cmdutil "k8s.io/kubernetes/cmd/kubeadm/app/cmd/util"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	kubeconfigphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/kubeconfig"

	"github.com/pkg/errors"
)

var (
	kubeconfigFilePhaseProperties = map[string]struct {
		name  string
		short string
		long  string
	}{
		kubeadmconstants.AdminKubeConfigFileName: {
			name:  "admin",
			short: "为管理员和kubeadm本身生成一个kubeconfig文件",
			long:  "为管理员和kubeadm本身生成kubeconfig文件，并将其保存到%s文件。",
		},
		kubeadmconstants.KubeletKubeConfigFileName: {
			name:  "kubelet",
			short: "为kubelet生成一个kubeconfig文件，以便仅*用于群集引导",
			long: cmdutil.LongDesc(`
					生成kubeconfig文件供kubelet使用，并将其保存到%s文件中。

					请注意，这应该仅用于群集引导。控制平面启动后，您应该从企业社会责任应用编程接口请求所有kubelet凭据。`),
		},
		kubeadmconstants.ControllerManagerKubeConfigFileName: {
			name:  "controller-manager",
			short: "生成一个kubeconfig文件供控制器管理器使用",
			long:  "生成kubeconfig文件供控制器管理器使用，并将其保存到%s文件中",
		},
		kubeadmconstants.SchedulerKubeConfigFileName: {
			name:  "scheduler",
			short: "生成一个kubeconfig文件供调度程序使用",
			long:  "生成kubeconfig文件供调度程序使用，并将其保存到%s文件。",
		},
	}
)

// NewKubeConfigPhase 创建kubeadm工作流阶段，该阶段创建建立控制平面和管理kubeconfig文件所需的所有kubeconfig文件。
func NewKubeConfigPhase() workflow.Phase {
	return workflow.Phase{
		Name:  "kubeconfig",
		Short: "生成全部的kubeconfig文件, 建立控制平面和管理必须的kubeconfig文件",
		Long:  cmdutil.MacroCommandLongDescription,
		Phases: []workflow.Phase{
			{
				Name:           "all",
				Short:          "生成全部的kubeconfig文件",
				InheritFlags:   getKubeConfigPhaseFlags("all"),
				RunAllSiblings: true,
			},
			NewKubeConfigFilePhase(kubeadmconstants.AdminKubeConfigFileName),
			NewKubeConfigFilePhase(kubeadmconstants.KubeletKubeConfigFileName),
			NewKubeConfigFilePhase(kubeadmconstants.ControllerManagerKubeConfigFileName),
			NewKubeConfigFilePhase(kubeadmconstants.SchedulerKubeConfigFileName),
		},
		Run: runKubeConfig,
	}
}

// NewKubeConfigFilePhase 创建kubeadm工作流阶段，该阶段创建kubeconfig文件。
func NewKubeConfigFilePhase(kubeConfigFileName string) workflow.Phase {
	return workflow.Phase{
		Name:         kubeconfigFilePhaseProperties[kubeConfigFileName].name,
		Short:        kubeconfigFilePhaseProperties[kubeConfigFileName].short,
		Long:         fmt.Sprintf(kubeconfigFilePhaseProperties[kubeConfigFileName].long, kubeConfigFileName),
		Run:          runKubeConfigFile(kubeConfigFileName),
		InheritFlags: getKubeConfigPhaseFlags(kubeConfigFileName),
	}
}

func getKubeConfigPhaseFlags(name string) []string {
	flags := []string{
		options.APIServerAdvertiseAddress,
		options.ControlPlaneEndpoint,
		options.APIServerBindPort,
		options.CertificatesDir,
		options.CfgPath,
		options.KubeconfigDir,
		options.KubernetesVersion,
	}
	if name == "all" || name == kubeadmconstants.KubeletKubeConfigFileName {
		flags = append(flags,
			options.NodeName,
		)
	}
	return flags
}

func runKubeConfig(c workflow.RunData) error {
	data, ok := c.(InitData)
	if !ok {
		return errors.New("使用无效的数据结构调用kubeconfig阶段")
	}

	fmt.Printf("[kubeconfig] 使用kubeconfig目录 %q\n", data.KubeConfigDir())
	return nil
}

// runKubeConfigFile executes kubeconfig creation logic.
func runKubeConfigFile(kubeConfigFileName string) func(workflow.RunData) error {
	return func(c workflow.RunData) error {
		data, ok := c.(InitData)
		if !ok {
			return errors.New("kubeconfig phase invoked with an invalid data struct")
		}

		// if external CA mode, skip certificate authority generation
		if data.ExternalCA() {
			fmt.Printf("[kubeconfig] External CA mode: Using user provided %s\n", kubeConfigFileName)
			// If using an external CA while dryrun, copy kubeconfig files to dryrun dir for later use
			if data.DryRun() {
				externalCAFile := filepath.Join(kubeadmconstants.KubernetesDir, kubeConfigFileName)
				fileInfo, _ := os.Stat(externalCAFile)
				contents, err := os.ReadFile(externalCAFile)
				if err != nil {
					return err
				}
				err = os.WriteFile(filepath.Join(data.KubeConfigDir(), kubeConfigFileName), contents, fileInfo.Mode())
				if err != nil {
					return err
				}
			}
			return nil
		}

		// if dryrunning, reads certificates from a temporary folder (and defer restore to the path originally specified by the user)
		cfg := data.Cfg()
		cfg.CertificatesDir = data.CertificateWriteDir()
		defer func() { cfg.CertificatesDir = data.CertificateDir() }()

		// creates the KubeConfig file (or use existing)
		return kubeconfigphase.CreateKubeConfigFile(kubeConfigFileName, data.KubeConfigDir(), data.Cfg())
	}
}
