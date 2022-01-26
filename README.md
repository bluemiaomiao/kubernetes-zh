# Kubernetes中文完全注释

> 基于Kubernetes 1.22版本，可以帮你快速学习Kubernetes源代码

## 开始学习

- 下载代码:

```shell
mkdir $GOPATH/src/k8s.io && cd $GOPATH/src/k8s.io
git clone https://github.com/bluemiaomiao/kubernetes-zh.git kubernetes
```

- 安装依赖并编译

```shell
cd kubernetes-zh && go mod download
make
ls -alh ./_output
```

