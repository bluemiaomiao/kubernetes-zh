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
	"io"

	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"

	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
)

// resetData 是用于重置阶段的接口
// cmd/reset.go 中的 resetData 类型必须满足此接口
type resetData interface {
	ForceReset() bool
	InputReader() io.Reader
	IgnorePreflightErrors() sets.String
	Cfg() *kubeadmapi.InitConfiguration
	Client() clientset.Interface
	AddDirsToClean(dirs ...string)
	CertificatesDir() string
	CRISocketPath() string
}
