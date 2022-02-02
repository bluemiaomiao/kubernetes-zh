package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"

	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmscheme "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/scheme"
	kubeadmapiv1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta3"
	"k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/validation"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	phases "k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/init"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	cmdutil "k8s.io/kubernetes/cmd/kubeadm/app/cmd/util"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/features"
	certsphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	kubeconfigphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/kubeconfig"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/apiclient"
	configutil "k8s.io/kubernetes/cmd/kubeadm/app/util/config"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"

	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

var (
	initDoneTempl = template.Must(template.New("init").Parse(dedent.Dedent(`
		您的Kubernetes控制面已经初始化完成!

		要开始使用集群，您需要作为普通用户运行以下命令:

		  mkdir -p $HOME/.kube
		  sudo cp -i {{.KubeConfigPath}} $HOME/.kube/config
		  sudo chown $(id -u):$(id -g) $HOME/.kube/config

		或者, 如果你是root用户, 那么可以运行:

		  export KUBECONFIG=/etc/kubernetes/admin.conf

		您现在应该在集群上部署一个Pod网络.
		使用下列选项之一运行 "kubectl apply-f[podnetwork].yaml":
		  https://kubernetes.io/docs/concepts/cluster-administration/addons/

		{{if .ControlPlaneEndpoint -}}
		{{if .UploadCerts -}}
		现在，您可以将在每个节点上运行以下命令，将任意数量的控制平面节点连接为根节点:

		  {{.joinControlPlaneCommand}}

		请注意，证书密钥允许访问群集敏感数据，请保密！
		作为保障措施, 上传的证书将在两小时内删除; 如有必要, 您可以使用
		"kubeadm init phase upload-certs --upload-certs" 来重新加载证书。

		{{else -}}
		现在，通过复制每个节点上的证书颁发机构和服务帐户密钥，然后以root用户身份运行以下命令，
		可以加入任意数量的控制平面节点:

		  {{.joinControlPlaneCommand}}

		{{end}}{{end}}然后，通过以根用户身份在每个节点上运行以下命令，可以加入任意数量的工作节点:

		{{.joinWorkerCommand}}
		`)))
)

// initOptions 定义kubeadm init通过标志公开的所有init选项。
// 请注意，此结构包括公共kubeadm config API，但此API支持的选项中只有一个子集将作为标志公开。
type initOptions struct {
	cfgPath                 string
	skipTokenPrint          bool
	dryRun                  bool
	kubeconfigDir           string
	kubeconfigPath          string
	featureGatesString      string
	ignorePreflightErrors   []string
	bto                     *options.BootstrapTokenOptions
	externalInitCfg         *kubeadmapiv1.InitConfiguration
	externalClusterCfg      *kubeadmapiv1.ClusterConfiguration
	uploadCerts             bool
	skipCertificateKeyPrint bool
	patchesDir              string
}

// 编译时断言本地数据对象满足阶段数据接口。
var _ phases.InitData = &initData{}

// initData 定义运行kubeadm init Workflow时使用的所有运行时信息；
// 此数据在Workflow中包含的所有阶段中共享。
type initData struct {
	cfg                     *kubeadmapi.InitConfiguration
	skipTokenPrint          bool
	dryRun                  bool
	kubeconfigDir           string
	kubeconfigPath          string
	ignorePreflightErrors   sets.String
	certificatesDir         string
	dryRunDir               string
	externalCA              bool
	client                  clientset.Interface
	outputWriter            io.Writer
	uploadCerts             bool
	skipCertificateKeyPrint bool
	patchesDir              string
}

// newCmdInit 返回kubeadm init命令
// 注意. initOptions作为参数公开，以允许对newInitOptions方法进行单元测试，该方法实现了所有命令选项验证逻辑
func newCmdInit(out io.Writer, initOptions *initOptions) *cobra.Command {
	fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit]...")
	// 如果没有指定初始化的选项, 那么创建一个新的初始化选项
	if initOptions == nil {
		initOptions = newInitOptions()
	}

	// 构建一个Runner来运行这个Workflow
	initRunner := workflow.NewRunner()

	// 构建了一个cobra.Command的结构体
	cmd := &cobra.Command{
		Use:   "init",
		Short: "运行此命令以设置Kubernetes控制平面",
		// Run函数与RunE函数都是运行命令的功能函数, 但是当功能执行错误之后, Run需要手动调用cmd.Help(), RunE函数会自动调用
		// 并且如果Run与RunE同时在cobra.Command结构体对象中实现, 那么RunE函数的调用优先级高于Run函数
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][RunE]")

			// 通过传入的命令行选项初始化initRunner的initData结构体
			c, err := initRunner.InitData(args)
			if err != nil {
				return err
			}

			data := c.(*initData)
			fmt.Printf("[初始化] 使用的Kubernetes版本: %s\n", data.cfg.KubernetesVersion)

			// 让初始化Runner运行起来
			if err := initRunner.Run(args); err != nil {
				return err
			}

			// 打印kubeadm join的链接
			return showJoinCommand(data, out)
		},
		Args: cobra.NoArgs,
	}

	// TODO: 猜测是将一些标志保存起来, 然后如果其他阶段产生了子任务, 可能会使用到这些标志
	// 添加标志到init命令中
	// init命令本地的标志最终可能会被自动为其他阶段生成的子命令使用
	fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][AddInitConfigFlags]")
	fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][AddClusterConfigFlags]")
	fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][AddInitOtherFlags]")
	fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][AddTokenFlag]")
	fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][AddTTLFlag]")
	fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][AddImageMetaFlags]")

	AddInitConfigFlags(cmd.Flags(), initOptions.externalInitCfg)
	AddClusterConfigFlags(cmd.Flags(), initOptions.externalClusterCfg, &initOptions.featureGatesString)
	AddInitOtherFlags(cmd.Flags(), initOptions)
	initOptions.bto.AddTokenFlag(cmd.Flags())
	initOptions.bto.AddTTLFlag(cmd.Flags())
	options.AddImageMetaFlags(cmd.Flags(), &initOptions.externalClusterCfg.ImageRepository)

	// 定义init命令未使用但最终可由其他阶段自动生成的子命令使用的附加标志
	fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][SetAdditionalFlags]")
	initRunner.SetAdditionalFlags(func(flags *flag.FlagSet) {
		fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][AddKubeConfigFlag]")
		fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][AddKubeConfigDirFlag]")
		fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][AddControlPlanExtraArgsFlags]")

		options.AddKubeConfigFlag(flags, &initOptions.kubeconfigPath)
		options.AddKubeConfigDirFlag(flags, &initOptions.kubeconfigDir)
		options.AddControlPlanExtraArgsFlags(flags, &initOptions.externalClusterCfg.APIServer.ExtraArgs, &initOptions.externalClusterCfg.ControllerManager.ExtraArgs, &initOptions.externalClusterCfg.Scheduler.ExtraArgs)
	})

	// 使用不同阶段(Phase)的任务初始化Workflow的Runner
	fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][AppendPhase] x 13")
	initRunner.AppendPhase(phases.NewPreflightPhase())
	initRunner.AppendPhase(phases.NewCertsPhase())
	initRunner.AppendPhase(phases.NewKubeConfigPhase())
	initRunner.AppendPhase(phases.NewKubeletStartPhase())
	initRunner.AppendPhase(phases.NewControlPlanePhase())
	initRunner.AppendPhase(phases.NewEtcdPhase())
	initRunner.AppendPhase(phases.NewWaitControlPlanePhase())
	initRunner.AppendPhase(phases.NewUploadConfigPhase())
	initRunner.AppendPhase(phases.NewUploadCertsPhase())
	initRunner.AppendPhase(phases.NewMarkControlPlanePhase())
	initRunner.AppendPhase(phases.NewBootstrapTokenPhase())
	initRunner.AppendPhase(phases.NewKubeletFinalizePhase())
	initRunner.AppendPhase(phases.NewAddonPhase())

	// 设置数据生成器函数, 在Runner整个Workflow或者单个阶段上都使用该函数
	fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][SetDataInitializer]")
	initRunner.SetDataInitializer(func(cmd *cobra.Command, args []string) (workflow.RunData, error) {
		data, err := newInitData(cmd, args, initOptions, out)
		if err != nil {
			return nil, err
		}
		// 如果跳过阶段的标志为空，请使用配置中的值
		if len(initRunner.Options.SkipPhases) == 0 {
			initRunner.Options.SkipPhases = data.cfg.SkipPhases
		}
		return data, nil
	})

	// 通过更改命令帮助、添加--skip phases标志和添加phases子命令，将运行程序绑定到kubeadm init命令
	fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit][BindToCommand]")
	initRunner.BindToCommand(cmd)

	fmt.Println("执行: cmd/kubeadm/app/cmd/init.go[newCmdInit]...Done")
	return cmd
}

// AddInitConfigFlags 将绑定到配置的初始化标志添加到指定的标志集
func AddInitConfigFlags(flagSet *flag.FlagSet, cfg *kubeadmapiv1.InitConfiguration) {
	flagSet.StringVar(
		&cfg.LocalAPIEndpoint.AdvertiseAddress, options.APIServerAdvertiseAddress, cfg.LocalAPIEndpoint.AdvertiseAddress,
		"The IP address the API Server will advertise it's listening on. If not set the default network interface will be used.",
	)
	flagSet.Int32Var(
		&cfg.LocalAPIEndpoint.BindPort, options.APIServerBindPort, cfg.LocalAPIEndpoint.BindPort,
		"Port for the API Server to bind to.",
	)
	flagSet.StringVar(
		&cfg.NodeRegistration.Name, options.NodeName, cfg.NodeRegistration.Name,
		`Specify the node name.`,
	)
	flagSet.StringVar(
		&cfg.CertificateKey, options.CertificateKey, "",
		"Key used to encrypt the control-plane certificates in the kubeadm-certs Secret.",
	)
	cmdutil.AddCRISocketFlag(flagSet, &cfg.NodeRegistration.CRISocket)
}

// AddClusterConfigFlags 将绑定到配置的群集标志添加到指定的标志集
func AddClusterConfigFlags(flagSet *flag.FlagSet, cfg *kubeadmapiv1.ClusterConfiguration, featureGatesString *string) {
	flagSet.StringVar(
		&cfg.Networking.ServiceSubnet, options.NetworkingServiceSubnet, cfg.Networking.ServiceSubnet,
		"Use alternative range of IP address for service VIPs.",
	)
	flagSet.StringVar(
		&cfg.Networking.PodSubnet, options.NetworkingPodSubnet, cfg.Networking.PodSubnet,
		"Specify range of IP addresses for the pod network. If set, the control plane will automatically allocate CIDRs for every node.",
	)
	flagSet.StringVar(
		&cfg.Networking.DNSDomain, options.NetworkingDNSDomain, cfg.Networking.DNSDomain,
		`Use alternative domain for services, e.g. "myorg.internal".`,
	)

	flagSet.StringVar(
		&cfg.ControlPlaneEndpoint, options.ControlPlaneEndpoint, cfg.ControlPlaneEndpoint,
		`Specify a stable IP address or DNS name for the control plane.`,
	)

	options.AddKubernetesVersionFlag(flagSet, &cfg.KubernetesVersion)

	flagSet.StringVar(
		&cfg.CertificatesDir, options.CertificatesDir, cfg.CertificatesDir,
		`The path where to save and store the certificates.`,
	)
	flagSet.StringSliceVar(
		&cfg.APIServer.CertSANs, options.APIServerCertSANs, cfg.APIServer.CertSANs,
		`Optional extra Subject Alternative Names (SANs) to use for the API Server serving certificate. Can be both IP addresses and DNS names.`,
	)
	options.AddFeatureGatesStringFlag(flagSet, featureGatesString)
}

// AddInitOtherFlags 将未绑定到配置文件的初始化标志添加到给定标志集
// 注意: cmd/kubeadm/app/apis/kubeadm/validation/validation.go 应该允许所有未绑定cfg对象的标志
func AddInitOtherFlags(flagSet *flag.FlagSet, initOptions *initOptions) {
	options.AddConfigFlag(flagSet, &initOptions.cfgPath)
	flagSet.StringSliceVar(
		&initOptions.ignorePreflightErrors, options.IgnorePreflightErrors, initOptions.ignorePreflightErrors,
		"A list of checks whose errors will be shown as warnings. Example: 'IsPrivilegedUser,Swap'. Value 'all' ignores errors from all checks.",
	)
	flagSet.BoolVar(
		&initOptions.skipTokenPrint, options.SkipTokenPrint, initOptions.skipTokenPrint,
		"Skip printing of the default bootstrap token generated by 'kubeadm init'.",
	)
	flagSet.BoolVar(
		&initOptions.dryRun, options.DryRun, initOptions.dryRun,
		"Don't apply any changes; just output what would be done.",
	)
	flagSet.BoolVar(
		&initOptions.uploadCerts, options.UploadCerts, initOptions.uploadCerts,
		"Upload control-plane certificates to the kubeadm-certs Secret.",
	)
	flagSet.BoolVar(
		&initOptions.skipCertificateKeyPrint, options.SkipCertificateKeyPrint, initOptions.skipCertificateKeyPrint,
		"Don't print the key used to encrypt the control-plane certificates.",
	)
	options.AddPatchesFlag(flagSet, &initOptions.patchesDir)
}

// newInitOptions 返回可用于创建init命令标志的结构。
func newInitOptions() *initOptions {
	// 通过应用默认值初始化公共kubeadm配置API
	externalInitCfg := &kubeadmapiv1.InitConfiguration{}
	kubeadmscheme.Scheme.Default(externalInitCfg)

	externalClusterCfg := &kubeadmapiv1.ClusterConfiguration{}
	kubeadmscheme.Scheme.Default(externalClusterCfg)

	// 为引导令牌相关标志创建options对象，并覆盖其默认值.Description
	bto := options.NewBootstrapTokenOptions()
	bto.Description = "kubeadm init生成的默认引导令牌"

	return &initOptions{
		externalInitCfg:    externalInitCfg,
		externalClusterCfg: externalClusterCfg,
		bto:                bto,
		kubeconfigDir:      kubeadmconstants.KubernetesDir,
		kubeconfigPath:     kubeadmconstants.GetAdminKubeConfigPath(),
		uploadCerts:        false,
	}
}

// newInitData returns a new initData struct to be used for the execution of the kubeadm init workflow.
// This func takes care of validating initOptions passed to the command, and then it converts
// options into the internal InitConfiguration type that is used as input all the phases in the kubeadm init workflow
func newInitData(cmd *cobra.Command, args []string, options *initOptions, out io.Writer) (*initData, error) {
	// Re-apply defaults to the public kubeadm API (this will set only values not exposed/not set as a flags)
	kubeadmscheme.Scheme.Default(options.externalInitCfg)
	kubeadmscheme.Scheme.Default(options.externalClusterCfg)

	// Validate standalone flags values and/or combination of flags and then assigns
	// validated values to the public kubeadm config API when applicable
	var err error
	if options.externalClusterCfg.FeatureGates, err = features.NewFeatureGate(&features.InitFeatureGates, options.featureGatesString); err != nil {
		return nil, err
	}

	if err = validation.ValidateMixedArguments(cmd.Flags()); err != nil {
		return nil, err
	}

	if err = options.bto.ApplyTo(options.externalInitCfg); err != nil {
		return nil, err
	}

	// Either use the config file if specified, or convert public kubeadm API to the internal InitConfiguration
	// and validates InitConfiguration
	cfg, err := configutil.LoadOrDefaultInitConfiguration(options.cfgPath, options.externalInitCfg, options.externalClusterCfg)
	if err != nil {
		return nil, err
	}

	ignorePreflightErrorsSet, err := validation.ValidateIgnorePreflightErrors(options.ignorePreflightErrors, cfg.NodeRegistration.IgnorePreflightErrors)
	if err != nil {
		return nil, err
	}
	// Also set the union of pre-flight errors to InitConfiguration, to provide a consistent view of the runtime configuration:
	cfg.NodeRegistration.IgnorePreflightErrors = ignorePreflightErrorsSet.List()

	// override node name from the command line option
	if options.externalInitCfg.NodeRegistration.Name != "" {
		cfg.NodeRegistration.Name = options.externalInitCfg.NodeRegistration.Name
	}

	if err := configutil.VerifyAPIServerBindAddress(cfg.LocalAPIEndpoint.AdvertiseAddress); err != nil {
		return nil, err
	}
	if err := features.ValidateVersion(features.InitFeatureGates, cfg.FeatureGates, cfg.KubernetesVersion); err != nil {
		return nil, err
	}

	// if dry running creates a temporary folder for saving kubeadm generated files
	dryRunDir := ""
	if options.dryRun {
		// the KUBEADM_INIT_DRYRUN_DIR environment variable allows overriding the dry-run temporary
		// directory from the command line. This makes it possible to run "kubeadm init" integration
		// tests without root.
		if dryRunDir, err = kubeadmconstants.CreateTempDirForKubeadm(os.Getenv("KUBEADM_INIT_DRYRUN_DIR"), "kubeadm-init-dryrun"); err != nil {
			return nil, errors.Wrap(err, "couldn't create a temporary directory")
		}
	}

	// Checks if an external CA is provided by the user (when the CA Cert is present but the CA Key is not)
	externalCA, err := certsphase.UsingExternalCA(&cfg.ClusterConfiguration)
	if externalCA {
		// In case the certificates signed by CA (that should be provided by the user) are missing or invalid,
		// returns, because kubeadm can't regenerate them without the CA Key
		if err != nil {
			return nil, errors.Wrapf(err, "invalid or incomplete external CA")
		}

		// Validate that also the required kubeconfig files exists and are invalid, because
		// kubeadm can't regenerate them without the CA Key
		kubeconfigDir := options.kubeconfigDir
		if err := kubeconfigphase.ValidateKubeconfigsForExternalCA(kubeconfigDir, cfg); err != nil {
			return nil, err
		}
	}

	// Checks if an external Front-Proxy CA is provided by the user (when the Front-Proxy CA Cert is present but the Front-Proxy CA Key is not)
	externalFrontProxyCA, err := certsphase.UsingExternalFrontProxyCA(&cfg.ClusterConfiguration)
	if externalFrontProxyCA {
		// In case the certificates signed by Front-Proxy CA (that should be provided by the user) are missing or invalid,
		// returns, because kubeadm can't regenerate them without the Front-Proxy CA Key
		if err != nil {
			return nil, errors.Wrapf(err, "invalid or incomplete external front-proxy CA")
		}
	}

	if options.uploadCerts && (externalCA || externalFrontProxyCA) {
		return nil, errors.New("can't use upload-certs with an external CA or an external front-proxy CA")
	}

	return &initData{
		cfg:                     cfg,
		certificatesDir:         cfg.CertificatesDir,
		skipTokenPrint:          options.skipTokenPrint,
		dryRun:                  options.dryRun,
		dryRunDir:               dryRunDir,
		kubeconfigDir:           options.kubeconfigDir,
		kubeconfigPath:          options.kubeconfigPath,
		ignorePreflightErrors:   ignorePreflightErrorsSet,
		externalCA:              externalCA,
		outputWriter:            out,
		uploadCerts:             options.uploadCerts,
		skipCertificateKeyPrint: options.skipCertificateKeyPrint,
		patchesDir:              options.patchesDir,
	}, nil
}

// UploadCerts returns Uploadcerts flag.
func (d *initData) UploadCerts() bool {
	return d.uploadCerts
}

// CertificateKey returns the key used to encrypt the certs.
func (d *initData) CertificateKey() string {
	return d.cfg.CertificateKey
}

// SetCertificateKey set the key used to encrypt the certs.
func (d *initData) SetCertificateKey(key string) {
	d.cfg.CertificateKey = key
}

// SkipCertificateKeyPrint returns the skipCertificateKeyPrint flag.
func (d *initData) SkipCertificateKeyPrint() bool {
	return d.skipCertificateKeyPrint
}

// Cfg returns initConfiguration.
func (d *initData) Cfg() *kubeadmapi.InitConfiguration {
	return d.cfg
}

// DryRun returns the DryRun flag.
func (d *initData) DryRun() bool {
	return d.dryRun
}

// SkipTokenPrint returns the SkipTokenPrint flag.
func (d *initData) SkipTokenPrint() bool {
	return d.skipTokenPrint
}

// IgnorePreflightErrors returns the IgnorePreflightErrors flag.
func (d *initData) IgnorePreflightErrors() sets.String {
	return d.ignorePreflightErrors
}

// CertificateWriteDir returns the path to the certificate folder or the temporary folder path in case of DryRun.
func (d *initData) CertificateWriteDir() string {
	if d.dryRun {
		return d.dryRunDir
	}
	return d.certificatesDir
}

// CertificateDir returns the CertificateDir as originally specified by the user.
func (d *initData) CertificateDir() string {
	return d.certificatesDir
}

// KubeConfigDir returns the path of the Kubernetes configuration folder or the temporary folder path in case of DryRun.
func (d *initData) KubeConfigDir() string {
	if d.dryRun {
		return d.dryRunDir
	}
	return d.kubeconfigDir
}

// KubeConfigPath 返回用于连接Kubernetes的kubeconfig文件的路径
func (d *initData) KubeConfigPath() string {
	if d.dryRun {
		d.kubeconfigPath = filepath.Join(d.dryRunDir, kubeadmconstants.AdminKubeConfigFileName)
	}
	return d.kubeconfigPath
}

// ManifestDir returns the path where manifest should be stored or the temporary folder path in case of DryRun.
func (d *initData) ManifestDir() string {
	if d.dryRun {
		return d.dryRunDir
	}
	return kubeadmconstants.GetStaticPodDirectory()
}

// KubeletDir returns path of the kubelet configuration folder or the temporary folder in case of DryRun.
func (d *initData) KubeletDir() string {
	if d.dryRun {
		return d.dryRunDir
	}
	return kubeadmconstants.KubeletRunDirectory
}

// ExternalCA returns true if an external CA is provided by the user.
func (d *initData) ExternalCA() bool {
	return d.externalCA
}

// OutputWriter returns the io.Writer used to write output to by this command.
func (d *initData) OutputWriter() io.Writer {
	return d.outputWriter
}

// Client returns a Kubernetes client to be used by kubeadm.
// This function is implemented as a singleton, thus avoiding to recreate the client when it is used by different phases.
// Important. This function must be called after the admin.conf kubeconfig file is created.
func (d *initData) Client() (clientset.Interface, error) {
	if d.client == nil {
		if d.dryRun {
			svcSubnetCIDR, err := kubeadmconstants.GetKubernetesServiceCIDR(d.cfg.Networking.ServiceSubnet, features.Enabled(d.cfg.FeatureGates, features.IPv6DualStack))
			if err != nil {
				return nil, errors.Wrapf(err, "unable to get internal Kubernetes Service IP from the given service CIDR (%s)", d.cfg.Networking.ServiceSubnet)
			}
			// If we're dry-running, we should create a faked client that answers some GETs in order to be able to do the full init flow and just logs the rest of requests
			dryRunGetter := apiclient.NewInitDryRunGetter(d.cfg.NodeRegistration.Name, svcSubnetCIDR.String())
			d.client = apiclient.NewDryRunClient(dryRunGetter, os.Stdout)
		} else {
			// If we're acting for real, we should create a connection to the API server and wait for it to come up
			var err error
			d.client, err = kubeconfigutil.ClientSetFromFile(d.KubeConfigPath())
			if err != nil {
				return nil, err
			}
		}
	}
	return d.client, nil
}

// Tokens 返回一个令牌字符串数组。
func (d *initData) Tokens() []string {
	tokens := []string{}
	for _, bt := range d.cfg.BootstrapTokens {
		tokens = append(tokens, bt.Token.String())
	}
	return tokens
}

// PatchesDir returns the folder where patches for components are stored
func (d *initData) PatchesDir() string {
	// If provided, make the flag value override the one in config.
	if len(d.patchesDir) > 0 {
		return d.patchesDir
	}
	if d.cfg.Patches != nil {
		return d.cfg.Patches.Directory
	}
	return ""
}

func printJoinCommand(out io.Writer, adminKubeConfigPath, token string, i *initData) error {
	joinControlPlaneCommand, err := cmdutil.GetJoinControlPlaneCommand(adminKubeConfigPath, token, i.CertificateKey(), i.skipTokenPrint, i.skipCertificateKeyPrint)
	if err != nil {
		return err
	}

	joinWorkerCommand, err := cmdutil.GetJoinWorkerCommand(adminKubeConfigPath, token, i.skipTokenPrint)
	if err != nil {
		return err
	}

	ctx := map[string]interface{}{
		"KubeConfigPath":          adminKubeConfigPath,
		"ControlPlaneEndpoint":    i.Cfg().ControlPlaneEndpoint,
		"UploadCerts":             i.uploadCerts,
		"joinControlPlaneCommand": joinControlPlaneCommand,
		"joinWorkerCommand":       joinWorkerCommand,
	}

	return initDoneTempl.Execute(out, ctx)
}

// showJoinCommand 在init中的所有阶段完成后打印join命令
func showJoinCommand(i *initData, out io.Writer) error {
	adminKubeConfigPath := i.KubeConfigPath()

	// 如果用户有多个令牌，则多次打印join命令
	for _, token := range i.Tokens() {
		if err := printJoinCommand(out, adminKubeConfigPath, token, i); err != nil {
			return errors.Wrap(err, "打印join命令失败")
		}
	}

	return nil
}
