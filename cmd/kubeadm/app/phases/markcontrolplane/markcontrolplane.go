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

package markcontrolplane

import (
	"fmt"

	"k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/apiclient"

	"k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
)

var labelsToAdd = []string{
	// TODO: remove this label:
	// https://github.com/kubernetes/kubeadm/issues/2200
	constants.LabelNodeRoleOldControlPlane,
	constants.LabelNodeRoleControlPlane,
	constants.LabelExcludeFromExternalLB,
}

// MarkControlPlane 污染控制平面并设置控制平面标签
func MarkControlPlane(client clientset.Interface, controlPlaneName string, taints []v1.Taint) error {
	// TODO:删除此“已弃用”修改并直接传递“标签加载”:
	// https://github.com/kubernetes/kubeadm/issues/2200
	labels := make([]string, len(labelsToAdd))
	copy(labels, labelsToAdd)
	labels[0] = constants.LabelNodeRoleOldControlPlane + "(deprecated)"

	fmt.Printf("[mark-control-plane] 通过添加标签将节点%s标记为控制平面: %v\n",
		controlPlaneName, labels)

	if len(taints) > 0 {
		taintStrs := []string{}
		for _, taint := range taints {
			taintStrs = append(taintStrs, taint.ToString())
		}
		fmt.Printf("[mark-control-plane] 通过添加污点将节点%s标记为控制平面 %v\n", controlPlaneName, taintStrs)
	}

	return apiclient.PatchNode(client, controlPlaneName, func(n *v1.Node) {
		markControlPlaneNode(n, taints)
	})
}

func taintExists(taint v1.Taint, taints []v1.Taint) bool {
	for _, t := range taints {
		if t == taint {
			return true
		}
	}

	return false
}

func markControlPlaneNode(n *v1.Node, taints []v1.Taint) {
	for _, label := range labelsToAdd {
		n.ObjectMeta.Labels[label] = ""
	}

	for _, nt := range n.Spec.Taints {
		if !taintExists(nt, taints) {
			taints = append(taints, nt)
		}
	}

	n.Spec.Taints = taints
}
