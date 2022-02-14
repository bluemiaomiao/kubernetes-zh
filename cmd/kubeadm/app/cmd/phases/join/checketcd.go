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
	"fmt"

	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	etcdphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/etcd"

	"github.com/pkg/errors"
)

// NewCheckEtcdPhase 是一个隐藏阶段，在控制平面准备阶段之后，引导库删除阶段之前运行，确保etcd是健康的
func NewCheckEtcdPhase() workflow.Phase {
	return workflow.Phase{
		Name:   "check-etcd",
		Run:    runCheckEtcdPhase,
		Hidden: true,
	}
}

func runCheckEtcdPhase(c workflow.RunData) error {
	data, ok := c.(JoinData)
	if !ok {
		return errors.New("check-etcd phase invoked with an invalid data struct")
	}

	// 如果这不是控制平面，则跳过
	if data.Cfg().ControlPlane == nil {
		return nil
	}

	cfg, err := data.InitCfg()
	if err != nil {
		return err
	}

	if cfg.Etcd.External != nil {
		fmt.Println("[check-etcd] 在外部模式下跳过etcd检查")
		return nil
	}

	fmt.Println("[check-etcd] Checking that the etcd cluster is healthy")

	// 检查etcd集群是否正常
	// 注意 这种检查以前无法实现，因为它需要admin.conf和所有连接到etcd的证书
	client, err := data.ClientSet()
	if err != nil {
		return err
	}

	return etcdphase.CheckLocalEtcdClusterStatus(client, &cfg.ClusterConfiguration)
}
