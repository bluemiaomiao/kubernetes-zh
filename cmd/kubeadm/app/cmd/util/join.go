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

package util

import (
	"bytes"
	"crypto/x509"
	"html/template"
	"strings"

	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/pubkeypin"

	"k8s.io/client-go/tools/clientcmd"
	clientcertutil "k8s.io/client-go/util/cert"

	"github.com/pkg/errors"
)

// join命令的字符串模板, 可以被填充解析为字符串
var joinCommandTemplate = template.Must(template.New("join").Parse(`` +
	`kubeadm join {{.ControlPlaneHostPort}} --token {{.Token}} \
	{{range $h := .CAPubKeyPins}}--discovery-token-ca-cert-hash {{$h}} {{end}}{{if .ControlPlane}}\
	--control-plane {{if .CertificateKey}}--certificate-key {{.CertificateKey}}{{end}}{{end}}`,
))

// GetJoinWorkerCommand returns the kubeadm join command for a given token and
// and Kubernetes cluster (the current cluster in the kubeconfig file)
func GetJoinWorkerCommand(kubeConfigFile, token string, skipTokenPrint bool) (string, error) {
	return getJoinCommand(kubeConfigFile, token, "", false, skipTokenPrint, false)
}

// GetJoinControlPlaneCommand 返回给定令牌和Kubernetes集群（kubeconfig文件中的当前集群）的kubeadm join命令
func GetJoinControlPlaneCommand(kubeConfigFile, token, key string, skipTokenPrint, skipCertificateKeyPrint bool) (string, error) {
	return getJoinCommand(kubeConfigFile, token, key, true, skipTokenPrint, skipCertificateKeyPrint)
}

func getJoinCommand(kubeConfigFile, token, key string, controlPlane, skipTokenPrint, skipCertificateKeyPrint bool) (string, error) {
	// 加载kubeconfig文件以获取CA证书和端点
	config, err := clientcmd.LoadFromFile(kubeConfigFile)
	if err != nil {
		return "", errors.Wrap(err, "未能加载kubeconfig")
	}

	// 加载默认的集群配置
	clusterConfig := kubeconfigutil.GetClusterFromKubeConfig(config)
	if clusterConfig == nil {
		return "", errors.New("无法获取默认群集配置")
	}

	// 从kubeconfig加载CA证书（从PEM数据或通过文件路径）
	// x509是Golang的内建库
	var caCerts []*x509.Certificate
	if clusterConfig.CertificateAuthorityData != nil {
		caCerts, err = clientcertutil.ParseCertsPEM(clusterConfig.CertificateAuthorityData)
		if err != nil {
			return "", errors.Wrap(err, "无法从kubeconfig解析CA证书")
		}
	} else if clusterConfig.CertificateAuthority != "" {
		caCerts, err = clientcertutil.CertsFromFile(clusterConfig.CertificateAuthority)
		if err != nil {
			return "", errors.Wrap(err, "无法加载kubeconfig引用的CA证书")
		}
	} else {
		return "", errors.New("在kubeconfig中未找到CA证书")
	}

	// 散列所有CA证书，并将其公钥PIN作为可信值包含在内
	publicKeyPins := make([]string, 0, len(caCerts))
	for _, caCert := range caCerts {
		publicKeyPins = append(publicKeyPins, pubkeypin.Hash(caCert))
	}

	ctx := map[string]interface{}{
		"Token":                token,
		"CAPubKeyPins":         publicKeyPins,
		"ControlPlaneHostPort": strings.Replace(clusterConfig.Server, "https://", "", -1),
		"CertificateKey":       key,
		"ControlPlane":         controlPlane,
	}

	if skipTokenPrint {
		ctx["Token"] = template.HTML("<value withheld>")
	}
	if skipCertificateKeyPrint {
		ctx["CertificateKey"] = template.HTML("<value withheld>")
	}

	var out bytes.Buffer
	err = joinCommandTemplate.Execute(&out, ctx)
	if err != nil {
		return "", errors.Wrap(err, "无法渲染join命令模板")
	}
	return out.String(), nil
}
