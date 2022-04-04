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
	"bufio"
	"errors"
	"fmt"
	"strings"

	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	"k8s.io/kubernetes/cmd/kubeadm/app/preflight"
)

// NewPreflightPhase 创建kubeadm工作流阶段，执行pre-flight前重置检查
func NewPreflightPhase() workflow.Phase {
	return workflow.Phase{
		Name:    "preflight",
		Aliases: []string{"pre-flight"},
		Short:   "运行重置操作的预检",
		Long:    "为 kubeadm reset 运行预检",
		Run:     runPreflight,
		InheritFlags: []string{
			options.IgnorePreflightErrors,
			options.ForceReset,
		},
	}
}

// runPreflight 执行预检逻辑
func runPreflight(c workflow.RunData) error {
	r, ok := c.(resetData)
	if !ok {
		return errors.New("用无效的数据结构调用了预检阶段")
	}

	if !r.ForceReset() {
		fmt.Println("[重置] 警告: kubeadm init 或 kubeadm join 对此主机所做的更改将被还原")
		fmt.Print("[重置] 确定要开始吗? [y/N]: ")

		s := bufio.NewScanner(r.InputReader())
		s.Scan()
		
		if err := s.Err(); err != nil {
			return err
		}
		if strings.ToLower(s.Text()) != "y" {
			return errors.New("中止重置操作")
		}
	}

	fmt.Println("[预检] 运行预检")
	return preflight.RunRootCheckOnly(r.IgnorePreflightErrors())
}
