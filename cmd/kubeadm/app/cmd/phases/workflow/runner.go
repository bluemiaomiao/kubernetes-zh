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
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// phaseSeparator 定义连接嵌套阶段名称时要使用的分隔符
const phaseSeparator = "/"

// RunnerOptions 定义执行kubeadm可组合工作流期间支持的选项
type RunnerOptions struct {
	// FilterPhases 定义要执行的阶段列表（如果为空, 则为全部）。
	FilterPhases []string

	// SkipPhases 定义要通过执行排除的阶段列表（如果为空, 则为无）。
	SkipPhases []string
}

// RunData 定义工作流中包括的所有阶段（即任何类型）之间共享的数据
type RunData = interface{}

// Runner 实现可组合kubeadm工作流的管理
type Runner struct {
	// Options 调节Runner的行为
	Options RunnerOptions

	// Phases 组成由Runner构成的Workflow
	Phases []Phase

	// runDataInitializer 在Workflow中包含的所有阶段中创建共享运行时数据
	runDataInitializer func(*cobra.Command, []string) (RunData, error)

	// runData 是Runner内部状态的一部分, 用于在InitData方法中实现单例(从而避免多次初始化数据)
	runData RunData

	// runCmd 是Runner内部状态的一部分, 用于跟踪触发Runner的命令(仅当Runner为BindToCommand时)
	runCmd *cobra.Command

	// cmdAdditionalFlags包含可以添加到为阶段生成的子命令中的其他共享标志。标志也可以从父命令继承，或直接添加到每个阶段
	cmdAdditionalFlags *pflag.FlagSet

	// phaseRunners是Runner内部状态的一部分，它为组成Workflow的阶段提供一个包装列表，其中包含支持阶段执行的上下文信息。
	phaseRunners []*phaseRunner
}

// phaseRunner 为一个阶段提供了一个包装器，添加了一组由Runner管理的Workflow派生的上下文信息。
// TODO: 如果我们决定变得更复杂，我们可以用定义良好的DAG或Tree库替换这种类型。
type phaseRunner struct {
	// 阶段提供对阶段实施的访问
	Phase

	// 提供对Runner管理的Workflow中父阶段的访问。
	parent *phaseRunner

	// 级别定义将此阶段嵌套到Runner管理的Workflow中的级别。
	level int

	// selfPath包含路径的所有元素，这些元素标识了Runner管理的工作流的阶段。
	selfPath []string

	// generatedName是阶段的全名，对应于Runner管理的工作流中阶段的绝对路径。
	generatedName string

	// use 是将在Workflow帮助中打印的阶段用法字符串。
	// 它对应于Runner管理的工作流中阶段的相对路径。
	use string
}

// NewRunner 返回可组合kubeadm工作流的新运行程序.
func NewRunner() *Runner {
	return &Runner{
		Phases: []Phase{},
	}
}

// AppendPhase 将给定阶段添加到Runner管理的有序阶段序列中。
func (e *Runner) AppendPhase(t Phase) {
	e.Phases = append(e.Phases, t)
}

// computePhaseRunFlags 返回一个映射，定义应该运行哪个阶段，不应该运行哪个阶段。
// PhaseRunFlags 根据RunnerOptions计算。
func (e *Runner) computePhaseRunFlags() (map[string]bool, error) {
	// 初始化支持数据结构
	phaseRunFlags := map[string]bool{}
	phaseHierarchy := map[string][]string{}
	_ = e.visitAll(func(p *phaseRunner) error {
		// 假设所有阶段都应该运行，初始化phaseRunFlags。
		phaseRunFlags[p.generatedName] = true

		// 为当前的阶段初始化 phaseHierarchy (取决于当前阶段的阶段列表)
		phaseHierarchy[p.generatedName] = []string{}

		// 将当前阶段注册为其自身父层次结构的一部分
		parent := p.parent
		for parent != nil {
			phaseHierarchy[parent.generatedName] = append(phaseHierarchy[parent.generatedName], p.generatedName)
			parent = parent.parent
		}
		return nil
	})

	// 如果指定了过滤器选项，则将所有phaseRunFlags设置为false，但过滤器中包含的阶段及其嵌套阶段的层次结构除外。
	if len(e.Options.FilterPhases) > 0 {
		for i := range phaseRunFlags {
			phaseRunFlags[i] = false
		}
		for _, f := range e.Options.FilterPhases {
			if _, ok := phaseRunFlags[f]; !ok {
				return phaseRunFlags, errors.Errorf("无效的阶段名称: %s", f)
			}
			phaseRunFlags[f] = true
			for _, c := range phaseHierarchy[f] {
				phaseRunFlags[c] = true
			}
		}
	}

	// 如果指定了“阶段跳过”选项，请将相应的phaseRunFlags设置为false，并对基础层次结构应用相同的更改
	for _, f := range e.Options.SkipPhases {
		if _, ok := phaseRunFlags[f]; !ok {
			return phaseRunFlags, errors.Errorf("无效的阶段名称: %s", f)
		}
		phaseRunFlags[f] = false
		for _, c := range phaseHierarchy[f] {
			phaseRunFlags[c] = false
		}
	}

	return phaseRunFlags, nil
}

// SetDataInitializer 允许设置初始化Workflow中所有阶段共享的运行时数据的函数。
// 该方法将在输入中接收触发运行程序的cmd（仅当Runner是BindToCommand时）
func (e *Runner) SetDataInitializer(builder func(cmd *cobra.Command, args []string) (RunData, error)) {
	e.runDataInitializer = builder
}

// InitData 触发在Workflow中包含的所有阶段之间共享运行时数据的创建
// 当需要在实际执行Run之前获取RunData时，或者在调用Run时隐式执行此操作。
func (e *Runner) InitData(args []string) (RunData, error) {
	// 确保runData是空的并且runData的初始化器已经创建成功
	if e.runData == nil && e.runDataInitializer != nil {
		var err error
		// 执行runData的初始化操作
		// 本质是传入用户定义的命令行参数并且执行创建Runner对象是挂载的func(*cobra.Command, []string) (RunData, error)函数
		if e.runData, err = e.runDataInitializer(e.runCmd, args); err != nil {
			return nil, err
		}
	}

	return e.runData, nil
}

// Run kubeadm可组合的kubeadm Workflow。
func (e *Runner) Run(args []string) error {
	e.prepareForExecution()

	// 根据RunnerOptions确定应该运行哪个阶段
	phaseRunFlags, err := e.computePhaseRunFlags()
	if err != nil {
		return err
	}

	// 构建Runner数据
	var data RunData
	if data, err = e.InitData(args); err != nil {
		return err
	}

	err = e.visitAll(func(p *phaseRunner) error {
		// 如果不应运行该阶段，请跳过该阶段。
		if run, ok := phaseRunFlags[p.generatedName]; !run || !ok {
			return nil
		}

		// 如果仅用于创建特殊子命令的阶段被错误地分配了运行方法，则会出现错误
		if p.RunAllSiblings && (p.RunIf != nil || p.Run != nil) {
			return errors.Errorf("标记为RunAllSides的阶段不能有运行函数 %s", p.generatedName)
		}

		// 如果阶段定义了在执行阶段操作之前要检查的条件。
		if p.RunIf != nil {
			// Check the condition and returns if the condition isn't satisfied (or fails)
			ok, err := p.RunIf(data)
			if err != nil {
				return errors.Wrapf(err, "阶段的错误执行运行条件 %s", p.generatedName)
			}

			if !ok {
				return nil
			}
		}

		// 运行阶段操作（如果已定义）
		if p.Run != nil {
			if err := p.Run(data); err != nil {
				return errors.Wrapf(err, "错误执行阶段 %s", p.generatedName)
			}
		}

		return nil
	})

	return err
}

// Help 返回包含Workflow中包含的阶段列表的文本。
func (e *Runner) Help(cmdUse string) string {
	e.prepareForExecution()

	// 计算每个阶段使用行的最大长度
	maxLength := 0
	_ = e.visitAll(func(p *phaseRunner) error {
		if !p.Hidden && !p.RunAllSiblings {
			length := len(p.use)
			if maxLength < length {
				maxLength = length
			}
		}
		return nil
	})

	// 打印按级别缩进并使用maxlength设置格式的阶段列表
	// 该列表包含在一个Markdown代码块中，以确保公共网站具有更好的可读性
	line := fmt.Sprintf("%q 命令执行以下阶段:\n", cmdUse)

	// 拼接Markdown文本
	line += "```\n"
	offset := 2
	_ = e.visitAll(func(p *phaseRunner) error {
		if !p.Hidden && !p.RunAllSiblings {
			padding := maxLength - len(p.use) + offset
			// 缩进
			line += strings.Repeat(" ", offset*p.level)
			// 名称+别名
			line += p.use
			// 填充至最大长度（+间距偏移）
			line += strings.Repeat(" ", padding)
			// 阶段简短描述
			line += p.Short
			line += "\n"
		}

		return nil
	})
	line += "```"
	// End:拼接Markdown文本

	return line
}

// SetAdditionalFlags 允许定义要添加的标志到为每个阶段生成的子命令（但不存在于父命令中）。
// 请注意，这个命令需要在BindToCommand之前完成。
// 注意: 如果只有一个阶段使用标志，请考虑使用阶段LoopFLAGS
func (e *Runner) SetAdditionalFlags(fn func(*pflag.FlagSet)) {
	// 创建一个新的NewFlagSet
	e.cmdAdditionalFlags = pflag.NewFlagSet("phaseAdditionalFlags", pflag.ContinueOnError)
	// 调用设置附加标志的函数
	fn(e.cmdAdditionalFlags)
}

// BindToCommand 通过更改命令帮助、添加阶段相关标志和添加阶段子命令，将Runner绑定到cobra命令
// 请注意，一旦所有阶段都添加到Runner中，就需要执行此命令。
func (e *Runner) BindToCommand(cmd *cobra.Command) {
	// 跟踪触发Runner的命令
	e.runCmd = cmd

	// 如果没有添加阶段，请提前返回
	if len(e.Phases) == 0 {
		return
	}

	e.prepareForExecution()

	// 添加阶段的子命令
	phaseCommand := &cobra.Command{
		Use:   "phase",
		Short: fmt.Sprintf("使用此命令可以调用 %s 工作流", cmd.Name()),
	}

	cmd.AddCommand(phaseCommand)

	// 生成用于调用单个阶段的所有嵌套子命令
	subcommands := map[string]*cobra.Command{}
	_ = e.visitAll(func(p *phaseRunner) error {
		// 跳过隐藏阶段
		if p.Hidden {
			return nil
		}

		// 初始化阶段选择器
		phaseSelector := p.generatedName

		// 如果请求，将阶段设置为运行所有同级阶段
		if p.RunAllSiblings {
			phaseSelector = p.parent.generatedName
		}

		// 创建阶段的子命令
		phaseCmd := &cobra.Command{
			Use:     strings.ToLower(p.Name),
			Short:   p.Short,
			Long:    p.Long,
			Example: p.Example,
			Aliases: p.Aliases,
			RunE: func(cmd *cobra.Command, args []string) error {
				// 如果阶段有子阶段，请打印帮助并退出
				if len(p.Phases) > 0 {
					return cmd.Help()
				}

				// 使用phaseCmd覆盖触发运行程序的命令
				e.runCmd = cmd
				e.Options.FilterPhases = []string{phaseSelector}
				return e.Run(args)
			},
		}

		// 使新命令从父命令继承本地标志
		// 注意: 全局标志将自动继承
		inheritsFlags(cmd.Flags(), phaseCmd.Flags(), p.InheritFlags)

		// 使新命令继承阶段的其他标志
		if e.cmdAdditionalFlags != nil {
			inheritsFlags(e.cmdAdditionalFlags, phaseCmd.Flags(), p.InheritFlags)
		}

		// 如果已定义，则添加阶段本地标志
		if p.LocalFlags != nil {
			p.LocalFlags.VisitAll(func(f *pflag.Flag) {
				phaseCmd.Flags().AddFlag(f)
			})
		}

		// 如果这个阶段有子对象（不是一个叶子节点），它不接受任何参数
		if len(p.Phases) > 0 {
			phaseCmd.Args = cobra.NoArgs
		} else {
			if p.ArgsValidator == nil {
				phaseCmd.Args = cmd.Args
			} else {
				phaseCmd.Args = p.ArgsValidator
			}
		}

		// 将命令添加到父级
		if p.level == 0 {
			phaseCommand.AddCommand(phaseCmd)
		} else {
			subcommands[p.parent.generatedName].AddCommand(phaseCmd)
		}

		subcommands[p.generatedName] = phaseCmd
		return nil
	})

	// 更改命令描述以显示可用阶段
	if cmd.Long != "" {
		cmd.Long = fmt.Sprintf("%s\n\n%s\n", cmd.Long, e.Help(cmd.Use))
	} else {
		cmd.Long = fmt.Sprintf("%s\n\n%s\n", cmd.Short, e.Help(cmd.Use))
	}

	// 向主命令添加与阶段相关的标志
	cmd.Flags().StringSliceVar(&e.Options.SkipPhases, "skip-phases", nil, "要跳过的阶段列表")
}

func inheritsFlags(sourceFlags, targetFlags *pflag.FlagSet, cmdFlags []string) {
	// 如果未定义要从父命令继承的标志列表，则不会添加任何标志
	if cmdFlags == nil {
		return
	}

	// 将要继承的所有标志添加到目标标志集
	sourceFlags.VisitAll(func(f *pflag.Flag) {
		for _, c := range cmdFlags {
			if f.Name == c {
				targetFlags.AddFlag(f)
			}
		}
	})
}

// visitAll 提供一种实用方法，用于按执行顺序访问Workflow中的所有阶段，并在每个阶段上执行传入的函数。
// Nested phase are visited immediately after their parent phase.
func (e *Runner) visitAll(fn func(*phaseRunner) error) error {
	for _, currentRunner := range e.phaseRunners {
		if err := fn(currentRunner); err != nil {
			return err
		}
	}
	return nil
}

// prepareForExecution 初始化Runner的内部状态（phaseRunner列表）。
func (e *Runner) prepareForExecution() {
	e.phaseRunners = []*phaseRunner{}
	var parentRunner *phaseRunner
	for _, phase := range e.Phases {
		// 跳过仅用于创建特殊子命令的阶段
		if phase.RunAllSiblings {
			continue
		}

		// 将阶段添加到执行列表中
		addPhaseRunner(e, parentRunner, phase)
	}
}

// addPhaseRunner 将给定阶段的phaseRunner添加到phaseRunner列表中
func addPhaseRunner(e *Runner, parentRunner *phaseRunner, phase Phase) {
	// 计算由Runner管理的Workflow派生的上下文信息。
	use := cleanName(phase.Name)
	generatedName := use
	selfPath := []string{generatedName}

	if parentRunner != nil {
		generatedName = strings.Join([]string{parentRunner.generatedName, generatedName}, phaseSeparator)
		use = fmt.Sprintf("%s%s", phaseSeparator, use)
		selfPath = append(parentRunner.selfPath, selfPath...)
	}

	// 创建phaseRunner
	currentRunner := &phaseRunner{
		Phase:         phase,
		parent:        parentRunner,
		level:         len(selfPath) - 1,
		selfPath:      selfPath,
		generatedName: generatedName,
		use:           use,
	}

	// 添加到phaseRunners列表中
	e.phaseRunners = append(e.phaseRunners, currentRunner)

	// 迭代嵌套、有序的阶段列表，从而以预期的执行顺序存储阶段（子阶段存储在其父阶段之后）。
	for _, childPhase := range phase.Phases {
		addPhaseRunner(e, currentRunner, childPhase)
	}
}

// cleanName 通过将名称小写并删除args描述符（如果有），使阶段名称适合runner帮助
func cleanName(name string) string {
	ret := strings.ToLower(name)
	if pos := strings.Index(ret, " "); pos != -1 {
		ret = ret[:pos]
	}
	return ret
}
