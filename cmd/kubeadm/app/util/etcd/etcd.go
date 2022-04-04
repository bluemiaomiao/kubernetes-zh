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

package etcd

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	"k8s.io/kubernetes/cmd/kubeadm/app/constants"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/pkg/errors"
	"go.etcd.io/etcd/client/pkg/v3/transport"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
)

const etcdTimeout = 2 * time.Second

// Exponential backoff for etcd operations (up to ~200 seconds)
var etcdBackoff = wait.Backoff{
	Steps:    18,
	Duration: 100 * time.Millisecond,
	Factor:   1.5,
	Jitter:   0.1,
}

// ClusterInterrogator is an interface to get etcd cluster related information
type ClusterInterrogator interface {
	CheckClusterHealth() error
	WaitForClusterAvailable(retries int, retryInterval time.Duration) (bool, error)
	Sync() error
	ListMembers() ([]Member, error)
	AddMember(name string, peerAddrs string) ([]Member, error)
	GetMemberID(peerURL string) (uint64, error)
	RemoveMember(id uint64) ([]Member, error)
}

// Client provides connection parameters for an etcd cluster
type Client struct {
	Endpoints []string
	TLS       *tls.Config
}

// New creates a new EtcdCluster client
func New(endpoints []string, ca, cert, key string) (*Client, error) {
	client := Client{Endpoints: endpoints}

	if ca != "" || cert != "" || key != "" {
		tlsInfo := transport.TLSInfo{
			CertFile:      cert,
			KeyFile:       key,
			TrustedCAFile: ca,
		}
		tlsConfig, err := tlsInfo.ClientConfig()
		if err != nil {
			return nil, err
		}
		client.TLS = tlsConfig
	}

	return &client, nil
}

// NewFromCluster 为 etcd 成员列表中的 etcd Endpoint 创建 etcd 客户端。
// 为了编写这些信息，它将首先发现至少一个要连接的 etcd Endpoint。
// 创建后，客户端将客户端的 Endpoint 与来自 etcd 成员资格 API 的已知 Endpoint 同步，因为它是可用成员列表的权威来源。
func NewFromCluster(client clientset.Interface, certificatesDir string) (*Client, error) {
	// 通过检查现有的 etcd Pod，发现至少一个要连接的 etcd Endpoint

	// 获取 etcd Endpoint 列表
	endpoints, err := getEtcdEndpoints(client)
	if err != nil {
		return nil, err
	}

	klog.V(1).Infof("从 Pod 读取的 etcd Endpoint: %s", strings.Join(endpoints, ","))

	// 创建一个 etcd 客户端
	etcdClient, err := New(
		endpoints,
		filepath.Join(certificatesDir, constants.EtcdCACertName),
		filepath.Join(certificatesDir, constants.EtcdHealthcheckClientCertName),
		filepath.Join(certificatesDir, constants.EtcdHealthcheckClientKeyName),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "为 %v 个 Endpoint 创建 etcd 客户端时出错", endpoints)
	}

	// 将客户端的 Endpoint 与 etcd 成员中的已知 Endpoint 同步。
	err = etcdClient.Sync()
	if err != nil {
		return nil, errors.Wrap(err, "error syncing endpoints with etcd")
	}
	klog.V(1).Infof("update etcd endpoints: %s", strings.Join(etcdClient.Endpoints, ","))

	return etcdClient, nil
}

// getEtcdEndpoints 返回 etcd 全部的 Endpoint
func getEtcdEndpoints(client clientset.Interface) ([]string, error) {
	return getEtcdEndpointsWithBackoff(client, constants.StaticPodMirroringDefaultRetry)
}

// getEtcdEndpointsWithBackoff 通过补偿的方式返回 etcd 全部的 Endpoint
func getEtcdEndpointsWithBackoff(client clientset.Interface, backoff wait.Backoff) ([]string, error) {
	return getRawEtcdEndpointsFromPodAnnotation(client, backoff)
}

// getRawEtcdEndpointsFromPodAnnotation 使用给定补偿返回 etcd Pod 注解上报告的 Endpoint 列表
func getRawEtcdEndpointsFromPodAnnotation(client clientset.Interface, backoff wait.Backoff) ([]string, error) {
	// etcd 的 Endpoint 列表
	etcdEndpoints := []string{}
	var lastErr error
	// 让我们容忍一些来自 API Server 或负载平衡器的意外瞬时故障。此外，如果静态 Pod 还没有被镜像到 API Server 中，我们希望等待这个传播过程。
	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		var overallEtcdPodCount int
		if etcdEndpoints, overallEtcdPodCount, lastErr = getRawEtcdEndpointsFromPodAnnotationWithoutRetry(client); lastErr != nil {
			return false, nil
		}
		if len(etcdEndpoints) == 0 || overallEtcdPodCount != len(etcdEndpoints) {
			klog.V(4).Infof("found a total of %d etcd pods and the following endpoints: %v; retrying",
				overallEtcdPodCount, etcdEndpoints)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		const message = "could not retrieve the list of etcd endpoints"
		if lastErr != nil {
			return []string{}, errors.Wrap(lastErr, message)
		}
		return []string{}, errors.Wrap(err, message)
	}
	return etcdEndpoints, nil
}

// getRawEtcdEndpointsFromPodAnnotationWithoutRetry 返回由 etcd Endpoint 注解报告的的 etcd Endpoint 列表，以及全局 etcd Endpoint 的数量。
// 这允许呼叫者区分“未找到 Endpoint ”和“未找到 Endpoint 且列出了 Pod”之间的区别，因此他们可以跳过重试。
func getRawEtcdEndpointsFromPodAnnotationWithoutRetry(client clientset.Interface) ([]string, int, error) {
	klog.V(3).Infof("从 etcd Endpoint 中的 %q 注解中检索 etcd Endpoint", constants.EtcdAdvertiseClientUrlsAnnotationKey)

	// 从 kube-system 这个名称空间获取
	podList, err := client.CoreV1().Pods(metav1.NamespaceSystem).List(
		context.TODO(),
		metav1.ListOptions{
			LabelSelector: fmt.Sprintf("组件=%s, 等级=%s", constants.Etcd, constants.ControlPlaneTier),
		},
	)
	if err != nil {
		return []string{}, 0, err
	}

	etcdEndpoints := []string{}
	for _, pod := range podList.Items {
		etcdEndpoint, ok := pod.ObjectMeta.Annotations[constants.EtcdAdvertiseClientUrlsAnnotationKey]
		if !ok {
			klog.V(3).Infof("etcd Pod %q 缺少 %q 注解；无法使用 Pod 注解推断 etcd 广播客户端的 URL", pod.ObjectMeta.Name, constants.EtcdAdvertiseClientUrlsAnnotationKey)
			continue
		}
		etcdEndpoints = append(etcdEndpoints, etcdEndpoint)
	}
	return etcdEndpoints, len(podList.Items), nil
}

// Sync 将客户端的 Endpoint 与 etcd 成员中的已知 Endpoint 同步。
func (c *Client) Sync() error {
	// 同步 Endpoint 列表
	var cli *clientv3.Client
	var lastError error
	err := wait.ExponentialBackoff(etcdBackoff, func() (bool, error) {
		var err error
		cli, err = clientv3.New(clientv3.Config{
			Endpoints:   c.Endpoints,
			DialTimeout: etcdTimeout,
			DialOptions: []grpc.DialOption{
				grpc.WithBlock(), // 阻塞，直到基础连接启动
			},
			TLS: c.TLS,
		})
		if err != nil {
			lastError = err
			return false, nil
		}

		// 处理客户端错误, 原来的代码没有处理
		defer func(cli *clientv3.Client) {
			_ = cli.Close()
		}(cli)

		ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)

		// 这才是真正的同步方法
		err = cli.Sync(ctx)

		cancel()
		if err == nil {
			return true, nil
		}

		klog.V(5).Infof("同步 etcd Endpoint 失败: %v", err)

		lastError = err
		return false, nil
	})

	if err != nil {
		return lastError
	}
	klog.V(1).Infof("从 etcd 读取的 etcd Endpoint: %s", strings.Join(cli.Endpoints(), ","))

	// 为客户端列出已经注册的全部 Endpoint
	c.Endpoints = cli.Endpoints()
	return nil
}

// Member struct defines an etcd member; it is used for avoiding to spread github.com/coreos/etcd dependency
// across kubeadm codebase
type Member struct {
	Name    string
	PeerURL string
}

func (c *Client) listMembers() (*clientv3.MemberListResponse, error) {
	// Gets the member list
	var lastError error
	var resp *clientv3.MemberListResponse
	err := wait.ExponentialBackoff(etcdBackoff, func() (bool, error) {
		cli, err := clientv3.New(clientv3.Config{
			Endpoints:   c.Endpoints,
			DialTimeout: etcdTimeout,
			DialOptions: []grpc.DialOption{
				grpc.WithBlock(), // block until the underlying connection is up
			},
			TLS: c.TLS,
		})
		if err != nil {
			lastError = err
			return false, nil
		}
		defer cli.Close()

		ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
		resp, err = cli.MemberList(ctx)
		cancel()
		if err == nil {
			return true, nil
		}
		klog.V(5).Infof("Failed to get etcd member list: %v", err)
		lastError = err
		return false, nil
	})
	if err != nil {
		return nil, lastError
	}
	return resp, nil
}

// GetMemberID returns the member ID of the given peer URL
func (c *Client) GetMemberID(peerURL string) (uint64, error) {
	resp, err := c.listMembers()
	if err != nil {
		return 0, err
	}

	for _, member := range resp.Members {
		if member.GetPeerURLs()[0] == peerURL {
			return member.GetID(), nil
		}
	}
	return 0, nil
}

// ListMembers returns the member list.
func (c *Client) ListMembers() ([]Member, error) {
	resp, err := c.listMembers()
	if err != nil {
		return nil, err
	}

	ret := make([]Member, 0, len(resp.Members))
	for _, m := range resp.Members {
		ret = append(ret, Member{Name: m.Name, PeerURL: m.PeerURLs[0]})
	}
	return ret, nil
}

// RemoveMember notifies an etcd cluster to remove an existing member
func (c *Client) RemoveMember(id uint64) ([]Member, error) {
	// Remove an existing member from the cluster
	var lastError error
	var resp *clientv3.MemberRemoveResponse
	err := wait.ExponentialBackoff(etcdBackoff, func() (bool, error) {
		cli, err := clientv3.New(clientv3.Config{
			Endpoints:   c.Endpoints,
			DialTimeout: etcdTimeout,
			DialOptions: []grpc.DialOption{
				grpc.WithBlock(), // block until the underlying connection is up
			},
			TLS: c.TLS,
		})
		if err != nil {
			lastError = err
			return false, nil
		}
		defer cli.Close()

		ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
		resp, err = cli.MemberRemove(ctx, id)
		cancel()
		if err == nil {
			return true, nil
		}
		klog.V(5).Infof("Failed to remove etcd member: %v", err)
		lastError = err
		return false, nil
	})
	if err != nil {
		return nil, lastError
	}

	// Returns the updated list of etcd members
	ret := []Member{}
	for _, m := range resp.Members {
		ret = append(ret, Member{Name: m.Name, PeerURL: m.PeerURLs[0]})
	}

	return ret, nil
}

// AddMember notifies an existing etcd cluster that a new member is joining
func (c *Client) AddMember(name string, peerAddrs string) ([]Member, error) {
	// Parse the peer address, required to add the client URL later to the list
	// of endpoints for this client. Parsing as a first operation to make sure that
	// if this fails no member addition is performed on the etcd cluster.
	parsedPeerAddrs, err := url.Parse(peerAddrs)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing peer address %s", peerAddrs)
	}

	// Adds a new member to the cluster
	var lastError error
	var resp *clientv3.MemberAddResponse
	err = wait.ExponentialBackoff(etcdBackoff, func() (bool, error) {
		cli, err := clientv3.New(clientv3.Config{
			Endpoints:   c.Endpoints,
			DialTimeout: etcdTimeout,
			DialOptions: []grpc.DialOption{
				grpc.WithBlock(), // block until the underlying connection is up
			},
			TLS: c.TLS,
		})
		if err != nil {
			lastError = err
			return false, nil
		}
		defer cli.Close()

		ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
		resp, err = cli.MemberAdd(ctx, []string{peerAddrs})
		cancel()
		if err == nil {
			return true, nil
		}
		klog.V(5).Infof("Failed to add etcd member: %v", err)
		lastError = err
		return false, nil
	})
	if err != nil {
		return nil, lastError
	}

	// Returns the updated list of etcd members
	ret := []Member{}
	for _, m := range resp.Members {
		// If the peer address matches, this is the member we are adding.
		// Use the name we passed to the function.
		if peerAddrs == m.PeerURLs[0] {
			ret = append(ret, Member{Name: name, PeerURL: peerAddrs})
			continue
		}
		// Otherwise, we are processing other existing etcd members returned by AddMembers.
		memberName := m.Name
		// In some cases during concurrent join, some members can end up without a name.
		// Use the member ID as name for those.
		if len(memberName) == 0 {
			memberName = strconv.FormatUint(m.ID, 16)
		}
		ret = append(ret, Member{Name: memberName, PeerURL: m.PeerURLs[0]})
	}

	// Add the new member client address to the list of endpoints
	c.Endpoints = append(c.Endpoints, GetClientURLByIP(parsedPeerAddrs.Hostname()))

	return ret, nil
}

// CheckClusterHealth returns nil for status Up or error for status Down
func (c *Client) CheckClusterHealth() error {
	_, err := c.getClusterStatus()
	return err
}

// getClusterStatus returns nil for status Up (along with endpoint status response map) or error for status Down
func (c *Client) getClusterStatus() (map[string]*clientv3.StatusResponse, error) {
	clusterStatus := make(map[string]*clientv3.StatusResponse)
	for _, ep := range c.Endpoints {
		// Gets the member status
		var lastError error
		var resp *clientv3.StatusResponse
		err := wait.ExponentialBackoff(etcdBackoff, func() (bool, error) {
			cli, err := clientv3.New(clientv3.Config{
				Endpoints:   c.Endpoints,
				DialTimeout: etcdTimeout,
				DialOptions: []grpc.DialOption{
					grpc.WithBlock(), // block until the underlying connection is up
				},
				TLS: c.TLS,
			})
			if err != nil {
				lastError = err
				return false, nil
			}
			defer cli.Close()

			ctx, cancel := context.WithTimeout(context.Background(), etcdTimeout)
			resp, err = cli.Status(ctx, ep)
			cancel()
			if err == nil {
				return true, nil
			}
			klog.V(5).Infof("Failed to get etcd status for %s: %v", ep, err)
			lastError = err
			return false, nil
		})
		if err != nil {
			return nil, lastError
		}

		clusterStatus[ep] = resp
	}
	return clusterStatus, nil
}

// WaitForClusterAvailable returns true if all endpoints in the cluster are available after retry attempts, an error is returned otherwise
func (c *Client) WaitForClusterAvailable(retries int, retryInterval time.Duration) (bool, error) {
	for i := 0; i < retries; i++ {
		if i > 0 {
			klog.V(1).Infof("[etcd] Waiting %v until next retry\n", retryInterval)
			time.Sleep(retryInterval)
		}
		klog.V(2).Infof("[etcd] attempting to see if all cluster endpoints (%s) are available %d/%d", c.Endpoints, i+1, retries)
		_, err := c.getClusterStatus()
		if err != nil {
			switch err {
			case context.DeadlineExceeded:
				klog.V(1).Infof("[etcd] Attempt timed out")
			default:
				klog.V(1).Infof("[etcd] Attempt failed with error: %v\n", err)
			}
			continue
		}
		return true, nil
	}
	return false, errors.New("timeout waiting for etcd cluster to be available")
}

// GetClientURL creates an HTTPS URL that uses the configured advertise
// address and client port for the API controller
func GetClientURL(localEndpoint *kubeadmapi.APIEndpoint) string {
	return "https://" + net.JoinHostPort(localEndpoint.AdvertiseAddress, strconv.Itoa(constants.EtcdListenClientPort))
}

// GetPeerURL creates an HTTPS URL that uses the configured advertise
// address and peer port for the API controller
func GetPeerURL(localEndpoint *kubeadmapi.APIEndpoint) string {
	return "https://" + net.JoinHostPort(localEndpoint.AdvertiseAddress, strconv.Itoa(constants.EtcdListenPeerPort))
}

// GetClientURLByIP creates an HTTPS URL based on an IP address
// and the client listening port.
func GetClientURLByIP(ip string) string {
	return "https://" + net.JoinHostPort(ip, strconv.Itoa(constants.EtcdListenClientPort))
}
