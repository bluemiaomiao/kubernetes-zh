/*
Copyright 2017 The Kubernetes Authors.

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

	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	cmdutil "k8s.io/kubernetes/cmd/kubeadm/app/cmd/util"
	"k8s.io/kubernetes/cmd/kubeadm/app/preflight"

	utilsexec "k8s.io/utils/exec"

	"github.com/pkg/errors"
)

var (
	preflightExample = cmdutil.Examples(`
		# Run pre-flight checks for kubeadm init using a config file.
		kubeadm init phase preflight --config kubeadm-config.yaml
		`)
)

// NewPreflightPhase 创建kubeadm工作流阶段，为新的控制平面节点实现预检检查。
func NewPreflightPhase() workflow.Phase {
	return workflow.Phase{
		Name:    "preflight",
		Short:   "运行 pre-flight checks",
		Long:    "为 kubeadm init 运行 pre-flight checks",
		Example: preflightExample,
		Run:     runPreflight,
		InheritFlags: []string{
			options.CfgPath,
			options.IgnorePreflightErrors,
		},
	}
}

// runPreflight 执行预检逻辑
func runPreflight(c workflow.RunData) error {
	data, ok := c.(InitData)
	if !ok {
		return errors.New("使用无效数据结构调用预检阶段")
	}

	fmt.Println("[预检] 执行预检")
	// 此处执行主机节点的检查
	if err := preflight.RunInitNodeChecks(utilsexec.New(), data.Cfg(), data.IgnorePreflightErrors(), false, false); err != nil {
		return err
	}

	if !data.DryRun() {
		fmt.Println("[预检] 提取设置Kubernetes集群所需的镜像")
		fmt.Println("[预检] 这可能需要一两分钟，具体取决于您的互联网连接速度")
		fmt.Println("[预检] 您也可以使用 kubeadm config images pull")
		if err := preflight.RunPullImagesCheck(utilsexec.New(), data.Cfg(), data.IgnorePreflightErrors()); err != nil {
			return err
		}
	} else {
		fmt.Println("[预检] 需要提取所需的镜像 (例如 kubeadm config images pull")
	}

	return nil
}
