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

package util

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	errorsutil "k8s.io/apimachinery/pkg/util/errors"
)

const (
	// DefaultErrorExitCode 定义失败操作的退出代码
	DefaultErrorExitCode = 1
	// PreFlightExitCode 定义预检操作的退出代码
	PreFlightExitCode = 2
	// ValidationExitCode 定义退出代码验证检查
	ValidationExitCode = 3
)

// fatal prints the message if set and then exits.
func fatal(msg string, code int) {
	if len(msg) > 0 {
		// add newline if needed
		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}

		fmt.Fprint(os.Stderr, msg)
	}
	os.Exit(code)
}

// CheckErr 打印友好的错误码到标准输出, 并返回一个非零的退出码
// 无法识别的错误将以"错误:"前缀打印。
// 此方法是所用命令的通用方法，可由非kubectl命令使用
func CheckErr(err error) {
	checkErr(err, fatal)
}

// preflightError allows us to know if the error is a preflight error or not
// defining the interface here avoids an import cycle of pulling in preflight into the util package
type preflightError interface {
	Preflight() bool
}

// checkErr 将给定的错误格式化为字符串，并使用该字符串和退出代码调用传递的handleErr函数
func checkErr(err error, handleErr func(string, int)) {

	var msg string
	if err != nil {
		msg = fmt.Sprintf("%s\n要查看此错误的堆栈跟踪，请使用--v=5或更高的值执行", err.Error())
		// 检查klog中的详细级别是否足够高，并打印堆栈跟踪
		f := flag.CommandLine.Lookup("v")
		if f != nil {
			// 假设“v”标志包含符合klog "Level"类型别名的可解析Int32，因此这里不会处理来自ParseInt的错误
			if v, e := strconv.ParseInt(f.Value.String(), 10, 32); e == nil {
				// https://git.k8s.io/community/contributors/devel/sig-instrumentation/logging.md
				// klog.V(5) - 跟踪级详细信息
				if v > 4 {
					msg = fmt.Sprintf("%+v", err)
				}
			}
		}
	}

	switch err.(type) {
	case nil:
		return
	case preflightError:
		handleErr(msg, PreFlightExitCode)
	case errorsutil.Aggregate:
		handleErr(msg, ValidationExitCode)

	default:
		handleErr(msg, DefaultErrorExitCode)
	}
}

// FormatErrMsg returns a human-readable string describing the slice of errors passed to the function
func FormatErrMsg(errs []error) string {
	var errMsg string
	for _, err := range errs {
		errMsg = fmt.Sprintf("%s\t- %s\n", errMsg, err.Error())
	}
	return errMsg
}
