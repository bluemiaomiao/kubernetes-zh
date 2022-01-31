/*
Copyright 2018 The Kubernetes Authors.

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

package workflow

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Phase 提供一个工作流阶段的实现，该阶段允许通过简单地实例化此类型的变量来创建新阶段。
type Phase struct {
	// 阶段的名称.
	// 阶段的名称应该在属于同一个Workflow的阶段或者是属于同一个父阶段的阶段中拥有唯一的名称
	Name string

	// 阶段的别名
	Aliases []string

	// 阶段的简短描述
	Short string

	// 阶段的详细描述
	Long string

	// 阶段的示例用法
	Example string

	// 用于定义是否在workflow中隐藏该阶段
	// 比如: kubeadm init工作流中的 PrintFileSifDry 运行阶段可能会对用户隐藏
	Hidden bool

	// Phases 定义嵌套的、有序的阶段序列。
	Phases []Phase

	// RunAllSiblings 允许将运行所有同级阶段的责任分配给某个阶段
	// 注意: 标记为RunAllSides的阶段不能有运行函数
	RunAllSiblings bool

	// Run 定义实现阶段操作的函数。
	// 建议执行类型断言，例如使用golang type switch来验证RunData类型。
	Run func(data RunData) error

	// RunIf 定义一个函数，该函数实现在执行阶段操作之前应检查的条件。
	// 如果此函数返回nil，则始终执行阶段操作。
	RunIf func(data RunData) (bool, error)

	// InheritFlags 定义为该阶段生成的cobra命令应从父命令中定义的本地标志或阶段Runner中定义的其他标志继承的标志列表。
	// If the values is not set or empty, no flags will be assigned to the command
	// 注意: 全局标志由嵌套的cobra命令自动继承
	InheritFlags []string

	// LocalFlags 定义应分配给为该阶段生成的cobra命令的标志列表
	// 注意: 如果两个或多个阶段具有相同的局部标志，请考虑使用父命令中的局部标志或在阶段Runner中定义的附加标志。
	LocalFlags *pflag.FlagSet

	// ArgsValidator 定义用于验证此阶段的参数的位置参数函数
	// 如果没有设置，阶段将采用顶级命令的参数。
	ArgsValidator cobra.PositionalArgs
}

// AppendPhase 将给定阶段添加到嵌套的有序阶段序列中。
func (t *Phase) AppendPhase(phase Phase) {
	t.Phases = append(t.Phases, phase)
}
