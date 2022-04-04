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
	"errors"
	"fmt"
	"os"

	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
)

// NewUpdateClusterStatus 创建一个 kubeadm Workflow 的 Phase 执行 update-cluster-status
func NewUpdateClusterStatus() workflow.Phase {
	return workflow.Phase{
		Name:  "update-cluster-status",
		Short: "在 ClusterStatus 对象中删除这个节点 (废弃)",
		Run:   runUpdateClusterStatus,
	}
}

func runUpdateClusterStatus(c workflow.RunData) error {
	r, ok := c.(resetData)
	if !ok {
		return errors.New("无效的数据结构调用了 update-cluster-status 阶段")
	}

	cfg := r.Cfg()
	if isControlPlane() && cfg != nil {
		fmt.Println("update-cluster-status 阶段是废弃的功能，在未来的代码中可能会被移除" +
			"目前它不执行任何操作")
	}
	return nil
}

// isControlPlane checks if a node is a control-plane node by looking up
// the kube-apiserver manifest file
func isControlPlane() bool {
	filepath := kubeadmconstants.GetStaticPodFilepath(kubeadmconstants.KubeAPIServer, kubeadmconstants.GetStaticPodDirectory())
	_, err := os.Stat(filepath)
	return !os.IsNotExist(err)
}
