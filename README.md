# Kubernetes中文完全注释

> 基于Kubernetes 1.22版本，可以帮你快速学习Kubernetes源代码

## 开始学习

- 安装编译环境

```shell
sudo apt install golang make build-ess* -y
```

- 下载代码:

```shell
mkdir $GOPATH/src/k8s.io && cd $GOPATH/src/k8s.io
git clone https://github.com/bluemiaomiao/kubernetes-zh.git kubernetes
```

- 安装依赖并编译

```shell
cd kubernetes
chmod +x ./fix-limit.sh  # Git克隆后文件权限发生变化可能会导致编译失败
./fix-limit.sh           # 执行脚本修复权限
go mod download
make
ls -alh ./_output
```

## 特殊的

- Windows下克隆本仓库请勿使用WSL, 这会导致符号链接失效。
