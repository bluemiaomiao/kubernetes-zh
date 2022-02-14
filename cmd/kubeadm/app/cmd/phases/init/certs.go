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
	"os"
	"path/filepath"
	"strings"

	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmscheme "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/scheme"
	kubeadmapiv1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta3"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	cmdutil "k8s.io/kubernetes/cmd/kubeadm/app/cmd/util"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	certsphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/pkiutil"

	"github.com/pkg/errors"
)

var (
	saKeyLongDesc = fmt.Sprintf(cmdutil.LongDesc(`
		生成用于签署服务帐户令牌的私钥及其公钥，并将它们保存到
		%s 和 %s 文件中.
		如果两个文件都已经存在，kubeadm将跳过生成步骤，将使用现有文件。
		`+cmdutil.AlphaDisclaimer), kubeadmconstants.ServiceAccountPrivateKeyName, kubeadmconstants.ServiceAccountPublicKeyName)

	genericLongDesc = cmdutil.LongDesc(`
		生成 %[1]s, 并将它们保存到 %[2]s.crt 和 %[2]s.key files.%[3]s

		如果这两个文件都已经存在，kubeadm将跳过生成步骤，并使用现有文件
		` + cmdutil.AlphaDisclaimer)
)

// NewCertsPhase 返回证书的阶段
func NewCertsPhase() workflow.Phase {
	return workflow.Phase{
		Name:   "certs",
		Short:  "证书生成",
		Phases: newCertSubPhases(),
		Run:    runCerts,
		Long:   cmdutil.MacroCommandLongDescription,
	}
}

// newCertSubPhases 返回certs阶段的子阶段
func newCertSubPhases() []workflow.Phase {
	subPhases := []workflow.Phase{}

	// 全部的子阶段
	allPhase := workflow.Phase{
		Name:           "all",
		Short:          "创建全部的证书",
		InheritFlags:   getCertPhaseFlags("all"),
		RunAllSiblings: true,
	}

	subPhases = append(subPhases, allPhase)

	// 此循环假设GetDefaultCertList()总是返回一个证书列表，该列表前面是对它们签名的CA。
	var lastCACert *certsphase.KubeadmCert
	for _, cert := range certsphase.GetDefaultCertList() {
		var phase workflow.Phase
		if cert.CAName == "" {
			phase = newCertSubPhase(cert, runCAPhase(cert))
			lastCACert = cert
		} else {
			phase = newCertSubPhase(cert, runCertPhase(cert, lastCACert))
		}
		subPhases = append(subPhases, phase)
	}

	// SA创建私有/公共密钥对，它根本不使用x509
	saPhase := workflow.Phase{
		Name:         "sa",
		Short:        "生成用于签署服务帐户令牌的私钥及其公钥",
		Long:         saKeyLongDesc,
		Run:          runCertsSa,
		InheritFlags: []string{options.CertificatesDir},
	}

	subPhases = append(subPhases, saPhase)

	return subPhases
}

func newCertSubPhase(certSpec *certsphase.KubeadmCert, run func(c workflow.RunData) error) workflow.Phase {
	phase := workflow.Phase{
		Name:  certSpec.Name,
		Short: fmt.Sprintf("生成 %s", certSpec.LongName),
		Long: fmt.Sprintf(
			genericLongDesc,
			certSpec.LongName,
			certSpec.BaseName,
			getSANDescription(certSpec),
		),
		Run:          run,
		InheritFlags: getCertPhaseFlags(certSpec.Name),
	}
	return phase
}

func getCertPhaseFlags(name string) []string {
	flags := []string{
		options.CertificatesDir,
		options.CfgPath,
		options.KubernetesVersion,
	}
	if name == "all" || name == "apiserver" {
		flags = append(flags,
			options.APIServerAdvertiseAddress,
			options.ControlPlaneEndpoint,
			options.APIServerCertSANs,
			options.NetworkingDNSDomain,
			options.NetworkingServiceSubnet,
		)
	}
	return flags
}

func getSANDescription(certSpec *certsphase.KubeadmCert) string {
	//Defaulted config we will use to get SAN certs
	defaultConfig := cmdutil.DefaultInitConfiguration()
	// GetAPIServerAltNames errors without an AdvertiseAddress; this is as good as any.
	defaultConfig.LocalAPIEndpoint = kubeadmapiv1.APIEndpoint{
		AdvertiseAddress: "127.0.0.1",
	}

	defaultInternalConfig := &kubeadmapi.InitConfiguration{}

	kubeadmscheme.Scheme.Default(defaultConfig)
	if err := kubeadmscheme.Scheme.Convert(defaultConfig, defaultInternalConfig, nil); err != nil {
		return ""
	}

	certConfig, err := certSpec.GetConfig(defaultInternalConfig)
	if err != nil {
		return ""
	}

	if len(certConfig.AltNames.DNSNames) == 0 && len(certConfig.AltNames.IPs) == 0 {
		return ""
	}
	// This mutates the certConfig, but we're throwing it after we construct the command anyway
	sans := []string{}

	for _, dnsName := range certConfig.AltNames.DNSNames {
		if dnsName != "" {
			sans = append(sans, dnsName)
		}
	}

	for _, ip := range certConfig.AltNames.IPs {
		sans = append(sans, ip.String())
	}
	return fmt.Sprintf("\n\nDefault SANs are %s", strings.Join(sans, ", "))
}

func runCertsSa(c workflow.RunData) error {
	data, ok := c.(InitData)
	if !ok {
		return errors.New("使用无效的数据结构调用certs阶段")
	}

	// 如果是外部CA模式，跳过服务帐户密钥生成
	if data.ExternalCA() {
		fmt.Printf("[证书] 使用现有SA密钥\n")
		return nil
	}

	// 创建新的服务账户密钥(或使用现有密钥)
	return certsphase.CreateServiceAccountKeyAndPublicKeyFiles(data.CertificateWriteDir(), data.Cfg().ClusterConfiguration.PublicKeyAlgorithm())
}

func runCerts(c workflow.RunData) error {
	data, ok := c.(InitData)
	if !ok {
		return errors.New("使用无效的数据结构调用certs阶段")
	}

	fmt.Printf("[证书] 使用证书文件夹 %q\n", data.CertificateWriteDir())

	// 如果在试运行时使用外部证书颁发机构，请将证书颁发机构证书复制到试运行目录以备后用
	if data.ExternalCA() && data.DryRun() {
		externalCAFile := filepath.Join(data.Cfg().CertificatesDir, kubeadmconstants.CACertName)
		fileInfo, _ := os.Stat(externalCAFile)
		contents, err := os.ReadFile(externalCAFile)
		if err != nil {
			return err
		}
		err = os.WriteFile(filepath.Join(data.CertificateWriteDir(), kubeadmconstants.CACertName), contents, fileInfo.Mode())
		if err != nil {
			return err
		}
	}
	return nil
}

func runCAPhase(ca *certsphase.KubeadmCert) func(c workflow.RunData) error {
	return func(c workflow.RunData) error {
		data, ok := c.(InitData)
		if !ok {
			return errors.New("使用无效的数据结构调用certs阶段")
		}

		// 如果使用外部etcd，跳过etcd证书颁发机构的生成
		if data.Cfg().Etcd.External != nil && ca.Name == "etcd-ca" {
			fmt.Printf("[证书] 外部etcd模式: 跳过 %s 证书颁发机构生成\n", ca.BaseName)
			return nil
		}

		if cert, err := pkiutil.TryLoadCertFromDisk(data.CertificateDir(), ca.BaseName); err == nil {
			certsphase.CheckCertificatePeriodValidity(ca.BaseName, cert)

			if _, err := pkiutil.TryLoadKeyFromDisk(data.CertificateDir(), ca.BaseName); err == nil {
				fmt.Printf("[证书] 使用已经存在的 %s 认证授权\n", ca.BaseName)
				return nil
			}
			fmt.Printf("[证书] 使用已经存在的 %s 无Key证书颁发机构\n", ca.BaseName)
			return nil
		}

		// 如果正在运行，请将证书颁发机构写入临时文件夹(并将恢复推迟到用户最初指定的路径)
		cfg := data.Cfg()
		cfg.CertificatesDir = data.CertificateWriteDir()
		defer func() { cfg.CertificatesDir = data.CertificateDir() }()

		// 创建新的证书颁发机构(或使用现有的)
		return certsphase.CreateCACertAndKeyFiles(ca, cfg)
	}
}

func runCertPhase(cert *certsphase.KubeadmCert, caCert *certsphase.KubeadmCert) func(c workflow.RunData) error {
	return func(c workflow.RunData) error {
		data, ok := c.(InitData)
		if !ok {
			return errors.New("certs phase invoked with an invalid data struct")
		}

		// if using external etcd, skips etcd certificates generation
		if data.Cfg().Etcd.External != nil && cert.CAName == "etcd-ca" {
			fmt.Printf("[certs] External etcd mode: Skipping %s certificate generation\n", cert.BaseName)
			return nil
		}

		if certData, intermediates, err := pkiutil.TryLoadCertChainFromDisk(data.CertificateDir(), cert.BaseName); err == nil {
			certsphase.CheckCertificatePeriodValidity(cert.BaseName, certData)

			caCertData, err := pkiutil.TryLoadCertFromDisk(data.CertificateDir(), caCert.BaseName)
			if err != nil {
				return errors.Wrapf(err, "couldn't load CA certificate %s", caCert.Name)
			}

			certsphase.CheckCertificatePeriodValidity(caCert.BaseName, caCertData)

			if err := pkiutil.VerifyCertChain(certData, intermediates, caCertData); err != nil {
				return errors.Wrapf(err, "[certs] certificate %s not signed by CA certificate %s", cert.BaseName, caCert.BaseName)
			}

			fmt.Printf("[certs] Using existing %s certificate and key on disk\n", cert.BaseName)
			return nil
		}

		// if dryrunning, write certificates to a temporary folder (and defer restore to the path originally specified by the user)
		cfg := data.Cfg()
		cfg.CertificatesDir = data.CertificateWriteDir()
		defer func() { cfg.CertificatesDir = data.CertificateDir() }()

		// create the new certificate (or use existing)
		return certsphase.CreateCertAndKeyFilesWithCA(cert, caCert, cfg)
	}
}
