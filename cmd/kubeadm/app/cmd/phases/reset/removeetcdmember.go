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
	"path/filepath"

	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	etcdphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/etcd"
	utilstaticpod "k8s.io/kubernetes/cmd/kubeadm/app/util/staticpod"

	"k8s.io/klog/v2"
)

// NewRemoveETCDMemberPhase 为 remove-etcd-member 创建一个 kubeadm Workflow 的 Phase
func NewRemoveETCDMemberPhase() workflow.Phase {
	return workflow.Phase{
		Name:  "remove-etcd-member",
		Short: "移除本地 etcd 成员",
		Long:  "移除控制平面节点的本地 etcd 成员",
		Run:   runRemoveETCDMemberPhase,
		InheritFlags: []string{
			options.KubeconfigPath,
		},
	}
}

func runRemoveETCDMemberPhase(c workflow.RunData) error {
	r, ok := c.(resetData)
	if !ok {
		return errors.New("无效的数据结构调用了 remove-etcd-member-phase 阶段")
	}
	cfg := r.Cfg()

	// 仅在使用本地 etcd 时清除 etcd 数据
	klog.V(1).Infoln("[重置] 检查 etcd 配置")
	// 获取的 etcd 的配置文件
	etcdManifestPath := filepath.Join(kubeadmconstants.KubernetesDir, kubeadmconstants.ManifestsSubDirName, "etcd.yaml")
	// 获取到 etcd 的数据目录
	etcdDataDir, err := getEtcdDataDir(etcdManifestPath, cfg)
	if err == nil {
		r.AddDirsToClean(etcdDataDir)
		if cfg != nil {
			if err := etcdphase.RemoveStackedEtcdMemberFromCluster(r.Client(), cfg); err != nil {
				klog.Warningf("[重置] 无法删除 etcd 成员: %v，请使用 etcdctl 手动删除此 etcd 成员", err)
			}
		}
	} else {
		fmt.Println("[重置] 没有发现 etcd 的配置。可能是外部的 etcd。")
		fmt.Println("[重置] 请手动重置 etcd 以防止进一步的问题")
	}

	return nil
}

// getEtcdDataDir 获取 etcd 的数据目录
func getEtcdDataDir(manifestPath string, cfg *kubeadmapi.InitConfiguration) (string, error) {
	const etcdVolumeName = "etcd-data"
	var dataDir string

	if cfg != nil && cfg.Etcd.Local != nil {
		return cfg.Etcd.Local.DataDir, nil
	}
	klog.Warningln("[重置] 没有 kubeadm 配置, 使用 etcd Pod 规范获取数据目录")

	// 获取到 etcd 的那个 Pod 资源描述
	etcdPod, err := utilstaticpod.ReadStaticPodFromDisk(manifestPath)
	if err != nil {
		return "", err
	}

	for _, volumeMount := range etcdPod.Spec.Volumes {
		if volumeMount.Name == etcdVolumeName {
			dataDir = volumeMount.HostPath.Path
			break
		}
	}
	if dataDir == "" {
		return dataDir, errors.New("无效的 etcd Pod 清单")
	}
	return dataDir, nil
}
