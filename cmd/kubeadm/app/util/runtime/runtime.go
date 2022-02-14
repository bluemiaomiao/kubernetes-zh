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

package util

import (
	"path/filepath"
	goruntime "runtime"
	"strings"

	"k8s.io/kubernetes/cmd/kubeadm/app/constants"

	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	utilsexec "k8s.io/utils/exec"

	"github.com/pkg/errors"
)

// ContainerRuntime 是一个使用容器运行时的接口
type ContainerRuntime interface {
	IsDocker() bool
	IsRunning() error
	ListKubeContainers() ([]string, error)
	RemoveContainers(containers []string) error
	PullImage(image string) error
	ImageExists(image string) (bool, error)
}

// CRIRuntime 是一个与CRI接口的结构
type CRIRuntime struct {
	exec      utilsexec.Interface
	criSocket string
}

// DockerRuntime 是一个与Docker守护程序接口的结构
type DockerRuntime struct {
	exec utilsexec.Interface
}

// NewContainerRuntime 设置并返回一个ContainerRuntime结构
func NewContainerRuntime(execer utilsexec.Interface, criSocket string) (ContainerRuntime, error) {
	var toolName string
	var runtime ContainerRuntime

	if criSocket != constants.DefaultDockerCRISocket {
		toolName = "crictl"
		// 不推荐使用/var/run/crio/crio.sock作为端点，
		// 请考虑使用完整的url格式 "unix:///var/run/crio/crio.sock"
		// 如果CRI Socket文件的路径是没问题的并且运行在非Windows系统上就返回unix://协议的链接
		if filepath.IsAbs(criSocket) && goruntime.GOOS != "windows" {
			criSocket = "unix://" + criSocket
		}
		runtime = &CRIRuntime{execer, criSocket}
	} else {
		toolName = "docker"
		runtime = &DockerRuntime{execer}
	}

	if _, err := execer.LookPath(toolName); err != nil {
		return nil, errors.Wrapf(err, "%s 对于容器运行时是必需的", toolName)
	}

	return runtime, nil
}

// IsDocker 如果运行库是docker，则返回true
func (runtime *CRIRuntime) IsDocker() bool {
	return false
}

// IsDocker 如果运行库是docker，则返回true
func (runtime *DockerRuntime) IsDocker() bool {
	return true
}

// IsRunning 检查运行时是否正在运行
func (runtime *CRIRuntime) IsRunning() error {
	if out, err := runtime.exec.Command("crictl", "-r", runtime.criSocket, "info").CombinedOutput(); err != nil {
		return errors.Wrapf(err, "容器运行时未运行: 输出: %s, 错误", string(out))
	}
	return nil
}

// IsRunning 检查运行时是否正在运行
func (runtime *DockerRuntime) IsRunning() error {
	if out, err := runtime.exec.Command("docker", "info").CombinedOutput(); err != nil {
		return errors.Wrapf(err, "容器运行时未运行: 输出: %s, 错误", string(out))
	}
	return nil
}

// ListKubeContainers lists running k8s CRI pods
func (runtime *CRIRuntime) ListKubeContainers() ([]string, error) {
	out, err := runtime.exec.Command("crictl", "-r", runtime.criSocket, "pods", "-q").CombinedOutput()
	if err != nil {
		return nil, errors.Wrapf(err, "output: %s, error", string(out))
	}
	pods := []string{}
	pods = append(pods, strings.Fields(string(out))...)
	return pods, nil
}

// ListKubeContainers lists running k8s containers
func (runtime *DockerRuntime) ListKubeContainers() ([]string, error) {
	output, err := runtime.exec.Command("docker", "ps", "-a", "--filter", "name=k8s_", "-q").CombinedOutput()
	return strings.Fields(string(output)), err
}

// RemoveContainers removes running k8s pods
func (runtime *CRIRuntime) RemoveContainers(containers []string) error {
	errs := []error{}
	for _, container := range containers {
		out, err := runtime.exec.Command("crictl", "-r", runtime.criSocket, "stopp", container).CombinedOutput()
		if err != nil {
			// don't stop on errors, try to remove as many containers as possible
			errs = append(errs, errors.Wrapf(err, "failed to stop running pod %s: output: %s, error", container, string(out)))
		} else {
			out, err = runtime.exec.Command("crictl", "-r", runtime.criSocket, "rmp", container).CombinedOutput()
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to remove running container %s: output: %s, error", container, string(out)))
			}
		}
	}
	return errorsutil.NewAggregate(errs)
}

// RemoveContainers removes running containers
func (runtime *DockerRuntime) RemoveContainers(containers []string) error {
	errs := []error{}
	for _, container := range containers {
		out, err := runtime.exec.Command("docker", "stop", container).CombinedOutput()
		if err != nil {
			// don't stop on errors, try to remove as many containers as possible
			errs = append(errs, errors.Wrapf(err, "failed to stop running container %s: output: %s, error", container, string(out)))
		} else {
			out, err = runtime.exec.Command("docker", "rm", "--volumes", container).CombinedOutput()
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to remove running container %s: output: %s, error", container, string(out)))
			}
		}
	}
	return errorsutil.NewAggregate(errs)
}

// PullImage pulls the image
func (runtime *CRIRuntime) PullImage(image string) error {
	var err error
	var out []byte
	for i := 0; i < constants.PullImageRetry; i++ {
		out, err = runtime.exec.Command("crictl", "-r", runtime.criSocket, "pull", image).CombinedOutput()
		if err == nil {
			return nil
		}
	}
	return errors.Wrapf(err, "output: %s, error", out)
}

// PullImage pulls the image
func (runtime *DockerRuntime) PullImage(image string) error {
	var err error
	var out []byte
	for i := 0; i < constants.PullImageRetry; i++ {
		out, err = runtime.exec.Command("docker", "pull", image).CombinedOutput()
		if err == nil {
			return nil
		}
	}
	return errors.Wrapf(err, "output: %s, error", out)
}

// ImageExists checks to see if the image exists on the system
func (runtime *CRIRuntime) ImageExists(image string) (bool, error) {
	err := runtime.exec.Command("crictl", "-r", runtime.criSocket, "inspecti", image).Run()
	return err == nil, nil
}

// ImageExists checks to see if the image exists on the system
func (runtime *DockerRuntime) ImageExists(image string) (bool, error) {
	err := runtime.exec.Command("docker", "inspect", image).Run()
	return err == nil, nil
}

// detectCRISocketImpl is separated out only for test purposes, DON'T call it directly, use DetectCRISocket instead
func detectCRISocketImpl(isSocket func(string) bool) (string, error) {
	foundCRISockets := []string{}
	knownCRISockets := []string{
		// Docker and containerd sockets are special cased below, hence not to be included here
		"/var/run/crio/crio.sock",
	}

	if isSocket(dockerSocket) {
		// the path in dockerSocket is not CRI compatible, hence we should replace it with a CRI compatible socket
		foundCRISockets = append(foundCRISockets, constants.DefaultDockerCRISocket)
	} else if isSocket(containerdSocket) {
		// Docker 18.09 gets bundled together with containerd, thus having both dockerSocket and containerdSocket present.
		// For compatibility reasons, we use the containerd socket only if Docker is not detected.
		foundCRISockets = append(foundCRISockets, containerdSocket)
	}

	for _, socket := range knownCRISockets {
		if isSocket(socket) {
			foundCRISockets = append(foundCRISockets, socket)
		}
	}

	switch len(foundCRISockets) {
	case 0:
		// Fall back to Docker if no CRI is detected, we can error out later on if we need it
		return constants.DefaultDockerCRISocket, nil
	case 1:
		// Precisely one CRI found, use that
		return foundCRISockets[0], nil
	default:
		// Multiple CRIs installed?
		return "", errors.Errorf("Found multiple CRI sockets, please use --cri-socket to select one: %s", strings.Join(foundCRISockets, ", "))
	}
}

// DetectCRISocket uses a list of known CRI sockets to detect one. If more than one or none is discovered, an error is returned.
func DetectCRISocket() (string, error) {
	return detectCRISocketImpl(isExistingSocket)
}
