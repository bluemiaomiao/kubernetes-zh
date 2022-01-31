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

package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	apimachineryversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/component-base/version"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// Version 提供kubeadm的版本信息
type Version struct {
	ClientVersion *apimachineryversion.Info `json:"clientVersion"`
}

// newCmdVersion 提供kubeadm的版本信息
func newCmdVersion(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "打印kubeadm的版本信息",
		Long:  "打印kubeadm相关的详细版本信息",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("执行: cmd/kubeadm/app/cmd/version.go[newCmdVersion][RunE]")
			return RunVersion(out, cmd)
		},
		Args: cobra.NoArgs,
	}
	cmd.Flags().StringP("output", "o", "", "输出格式, 可用的选项有: 'yaml', 'json' and 'short'")
	return cmd
}

// RunVersion 提供kubeadm的版本信息，格式取决于cobra.Command中指定的参数
func RunVersion(out io.Writer, cmd *cobra.Command) error {
	fmt.Println("执行: cmd/kubeadm/app/cmd/version.go[newCmdVersion][RunVersion]")
	klog.V(1).Infoln("[版本] 正在检索版本信息")
	// 返回整个代码基版本, 它是用来检测二进制代码是用什么代码构建的。
	clientVersion := version.Get()

	v := Version{
		ClientVersion: &clientVersion,
	}

	// TODO: !!!修复使用手动编译方式造成Shell脚本无法映射版本信息到Go文件致使kubeadm运行失败的问题
	v.ClientVersion.Major = "1"
	v.ClientVersion.Minor = "22"
	v.ClientVersion.GitVersion = "v1.22.6"
	v.ClientVersion.GitCommit = "f59f5c2fda36e4036b49ec027e556a15456108f0"
	v.ClientVersion.GitTreeState = "clean"
	v.ClientVersion.BuildDate = "2022-01-19T17:31:49Z"
	v.ClientVersion.GoVersion = "go1.17"
	v.ClientVersion.Compiler = "gc"
	v.ClientVersion.Platform = "linux/amd64"

	const flag = "output"
	of, err := cmd.Flags().GetString(flag)
	if err != nil {
		return errors.Wrapf(err, "访问标志时出错 %s 在执行 %s 时", flag, cmd.Name())
	}

	switch of {
	case "":
		_, _ = fmt.Fprintf(out, "kubeadm 版本: %#v\n", v.ClientVersion)
	case "short":
		_, _ = fmt.Fprintf(out, "%s\n", v.ClientVersion.GitVersion)
	case "yaml":
		y, err := yaml.Marshal(&v)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(y))
	case "json":
		y, err := json.MarshalIndent(&v, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(y))
	default:
		return errors.Errorf("输出格式无效: %s", of)
	}

	return nil
}
