/*
Copyright 2016 The Kubernetes Authors.

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

package main

import (
	"k8s.io/kubernetes/cmd/kubeadm/app"
	kubeadmutil "k8s.io/kubernetes/cmd/kubeadm/app/util"
)

// kubeadm 二进制文件的主函数
func main() {
	// CheckErr 用于将 app.Run() 返回的 error 类型的值转换为字符串并打印出来
	kubeadmutil.CheckErr(app.Run())
}
