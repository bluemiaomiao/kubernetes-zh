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

// SetDataInitializer allows to setup a function that initialize the runtime data shared
// among all the phases included in the workflow.
// The method will receive in input the cmd that triggers the Runner (only if the runner is BindToCommand)
func (e *Runner) SetDataInitializer(builder func(cmd *cobra.Command, args []string) (RunData, error)) {
	e.runDataInitializer = builder
}

// InitData 触发在工作流中包含的所有阶段之间共享运行时数据的创建
// 当需要在实际执行Run之前获取RunData时，或者在调用Run时隐式执行此操作。
func (e *Runner) InitData(args []string) (RunData, error) {
	if e.runData == nil && e.runDataInitializer != nil {
		var err error
		if e.runData, err = e.runDataInitializer(e.runCmd, args); err != nil {
			return nil, err
		}
	}

	return e.runData, nil
}

// Run the kubeadm composable kubeadm workflows.
func (e *Runner) Run(args []string) error {
	e.prepareForExecution()

	// determine which phase should be run according to RunnerOptions
	phaseRunFlags, err := e.computePhaseRunFlags()
	if err != nil {
		return err
	}

	// builds the runner data
	var data RunData
	if data, err = e.InitData(args); err != nil {
		return err
	}

	err = e.visitAll(func(p *phaseRunner) error {
		// if the phase should not be run, skip the phase.
		if run, ok := phaseRunFlags[p.generatedName]; !run || !ok {
			return nil
		}

		// Errors if phases that are meant to create special subcommands only
		// are wrongly assigned Run Methods
		if p.RunAllSiblings && (p.RunIf != nil || p.Run != nil) {
			return errors.Errorf("phase marked as RunAllSiblings can not have Run functions %s", p.generatedName)
		}

		// If the phase defines a condition to be checked before executing the phase action.
		if p.RunIf != nil {
			// Check the condition and returns if the condition isn't satisfied (or fails)
			ok, err := p.RunIf(data)
			if err != nil {
				return errors.Wrapf(err, "error execution run condition for phase %s", p.generatedName)
			}

			if !ok {
				return nil
			}
		}

		// Runs the phase action (if defined)
		if p.Run != nil {
			if err := p.Run(data); err != nil {
				return errors.Wrapf(err, "error execution phase %s", p.generatedName)
			}
		}

		return nil
	})

	return err
}

// Help returns text with the list of phases included in the workflow.
func (e *Runner) Help(cmdUse string) string {
	e.prepareForExecution()

	// computes the max length of for each phase use line
	maxLength := 0
	e.visitAll(func(p *phaseRunner) error {
		if !p.Hidden && !p.RunAllSiblings {
			length := len(p.use)
			if maxLength < length {
				maxLength = length
			}
		}
		return nil
	})

	// prints the list of phases indented by level and formatted using the maxlength
	// the list is enclosed in a mardown code block for ensuring better readability in the public web site
	line := fmt.Sprintf("The %q command executes the following phases:\n", cmdUse)
	line += "```\n"
	offset := 2
	e.visitAll(func(p *phaseRunner) error {
		if !p.Hidden && !p.RunAllSiblings {
			padding := maxLength - len(p.use) + offset
			line += strings.Repeat(" ", offset*p.level) // indentation
			line += p.use                               // name + aliases
			line += strings.Repeat(" ", padding)        // padding right up to max length (+ offset for spacing)
			line += p.Short                             // phase short description
			line += "\n"
		}

		return nil
	})
	line += "```"
	return line
}

// SetAdditionalFlags allows to define flags to be added
// to the subcommands generated for each phase (but not existing in the parent command).
// Please note that this command needs to be done before BindToCommand.
// Nb. if a flag is used only by one phase, please consider using phase LocalFlags.
func (e *Runner) SetAdditionalFlags(fn func(*pflag.FlagSet)) {
	// creates a new NewFlagSet
	e.cmdAdditionalFlags = pflag.NewFlagSet("phaseAdditionalFlags", pflag.ContinueOnError)
	// invokes the function that sets additional flags
	fn(e.cmdAdditionalFlags)
}

// BindToCommand bind the Runner to a cobra command by altering
// command help, adding phase related flags and by adding phases subcommands
// Please note that this command needs to be done once all the phases are added to the Runner.
func (e *Runner) BindToCommand(cmd *cobra.Command) {
	// keep track of the command triggering the runner
	e.runCmd = cmd

	// return early if no phases were added
	if len(e.Phases) == 0 {
		return
	}

	e.prepareForExecution()

	// adds the phases subcommand
	phaseCommand := &cobra.Command{
		Use:   "phase",
		Short: fmt.Sprintf("Use this command to invoke single phase of the %s workflow", cmd.Name()),
	}

	cmd.AddCommand(phaseCommand)

	// generate all the nested subcommands for invoking single phases
	subcommands := map[string]*cobra.Command{}
	e.visitAll(func(p *phaseRunner) error {
		// skip hidden phases
		if p.Hidden {
			return nil
		}

		// initialize phase selector
		phaseSelector := p.generatedName

		// if requested, set the phase to run all the sibling phases
		if p.RunAllSiblings {
			phaseSelector = p.parent.generatedName
		}

		// creates phase subcommand
		phaseCmd := &cobra.Command{
			Use:     strings.ToLower(p.Name),
			Short:   p.Short,
			Long:    p.Long,
			Example: p.Example,
			Aliases: p.Aliases,
			RunE: func(cmd *cobra.Command, args []string) error {
				// if the phase has subphases, print the help and exits
				if len(p.Phases) > 0 {
					return cmd.Help()
				}

				// overrides the command triggering the Runner using the phaseCmd
				e.runCmd = cmd
				e.Options.FilterPhases = []string{phaseSelector}
				return e.Run(args)
			},
		}

		// makes the new command inherits local flags from the parent command
		// Nb. global flags will be inherited automatically
		inheritsFlags(cmd.Flags(), phaseCmd.Flags(), p.InheritFlags)

		// makes the new command inherits additional flags for phases
		if e.cmdAdditionalFlags != nil {
			inheritsFlags(e.cmdAdditionalFlags, phaseCmd.Flags(), p.InheritFlags)
		}

		// If defined, added phase local flags
		if p.LocalFlags != nil {
			p.LocalFlags.VisitAll(func(f *pflag.Flag) {
				phaseCmd.Flags().AddFlag(f)
			})
		}

		// if this phase has children (not a leaf) it doesn't accept any args
		if len(p.Phases) > 0 {
			phaseCmd.Args = cobra.NoArgs
		} else {
			if p.ArgsValidator == nil {
				phaseCmd.Args = cmd.Args
			} else {
				phaseCmd.Args = p.ArgsValidator
			}
		}

		// adds the command to parent
		if p.level == 0 {
			phaseCommand.AddCommand(phaseCmd)
		} else {
			subcommands[p.parent.generatedName].AddCommand(phaseCmd)
		}

		subcommands[p.generatedName] = phaseCmd
		return nil
	})

	// alters the command description to show available phases
	if cmd.Long != "" {
		cmd.Long = fmt.Sprintf("%s\n\n%s\n", cmd.Long, e.Help(cmd.Use))
	} else {
		cmd.Long = fmt.Sprintf("%s\n\n%s\n", cmd.Short, e.Help(cmd.Use))
	}

	// adds phase related flags to the main command
	cmd.Flags().StringSliceVar(&e.Options.SkipPhases, "skip-phases", nil, "List of phases to be skipped")
}

func inheritsFlags(sourceFlags, targetFlags *pflag.FlagSet, cmdFlags []string) {
	// If the list of flag to be inherited from the parent command is not defined, no flag is added
	if cmdFlags == nil {
		return
	}

	// add all the flags to be inherited to the target flagSet
	sourceFlags.VisitAll(func(f *pflag.Flag) {
		for _, c := range cmdFlags {
			if f.Name == c {
				targetFlags.AddFlag(f)
			}
		}
	})
}

// visitAll provides a utility method for visiting all the phases in the workflow
// in the execution order and executing a func on each phase.
// Nested phase are visited immediately after their parent phase.
func (e *Runner) visitAll(fn func(*phaseRunner) error) error {
	for _, currentRunner := range e.phaseRunners {
		if err := fn(currentRunner); err != nil {
			return err
		}
	}
	return nil
}

// prepareForExecution initialize the internal state of the Runner (the list of phaseRunner).
func (e *Runner) prepareForExecution() {
	e.phaseRunners = []*phaseRunner{}
	var parentRunner *phaseRunner
	for _, phase := range e.Phases {
		// skips phases that are meant to create special subcommands only
		if phase.RunAllSiblings {
			continue
		}

		// add phases to the execution list
		addPhaseRunner(e, parentRunner, phase)
	}
}

// addPhaseRunner adds the phaseRunner for a given phase to the phaseRunners list
func addPhaseRunner(e *Runner, parentRunner *phaseRunner, phase Phase) {
	// computes contextual information derived by the workflow managed by the Runner.
	use := cleanName(phase.Name)
	generatedName := use
	selfPath := []string{generatedName}

	if parentRunner != nil {
		generatedName = strings.Join([]string{parentRunner.generatedName, generatedName}, phaseSeparator)
		use = fmt.Sprintf("%s%s", phaseSeparator, use)
		selfPath = append(parentRunner.selfPath, selfPath...)
	}

	// creates the phaseRunner
	currentRunner := &phaseRunner{
		Phase:         phase,
		parent:        parentRunner,
		level:         len(selfPath) - 1,
		selfPath:      selfPath,
		generatedName: generatedName,
		use:           use,
	}

	// adds to the phaseRunners list
	e.phaseRunners = append(e.phaseRunners, currentRunner)

	// iterate for the nested, ordered list of phases, thus storing
	// phases in the expected executing order (child phase are stored immediately after their parent phase).
	for _, childPhase := range phase.Phases {
		addPhaseRunner(e, currentRunner, childPhase)
	}
}

// cleanName makes phase name suitable for the runner help, by lowercasing the name
// and removing args descriptors, if any
func cleanName(name string) string {
	ret := strings.ToLower(name)
	if pos := strings.Index(ret, " "); pos != -1 {
		ret = ret[:pos]
	}
	return ret
}
