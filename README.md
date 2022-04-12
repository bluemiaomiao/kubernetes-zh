# Kubernetes 中文完全注释

基于 Kubernetes 1.22版本，可以帮你快速学习 Kubernetes 源代码。

## 使用 Ubuntu （Ubuntu Server） 开始学习

> 此方式适用于云服务器、本地虚拟机、物理机。

> 注意: Windows 或者 Mac 平台推荐使用此方式。

- 安装编译环境

```shell
sudo apt install golang make build-ess* -y
```

- 下载代码

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
make -j 8
ls -alh ./_output
```

## 特殊的

- Windows 下克隆本仓库请勿使用 WSL, 这会导致符号链接失效。

## 使用 Docker 开始学习

- 将源代码存储库克隆到本地

```shell
git clone https://github.com/bluemiaomiao/kubernetes-zh.git kubernetes
```

- 启动一个编译环境

```shell
docker build -f Dockerfile -t kubernetes-dev:1.22 .
docker run --name kubernetes-dev0 -i -t -d -p 8022:22 -v <YourLocalMachineGOPATH>:/home/remote/go kubernetes-dev:1.22 /bin/bash
docker attach kubernetes-dev0 /bin/bash
```

- 进入 Docker 容器环境以后:

```shell
su - remote
cd $GOPATH/src/k8s.io/kubernetes
go mod download
chmod +x fix-limit.sh
./fix-limit.sh
make -j 8
```

Docker 容器将 SSH 端口映射到本机的8022端口, 你可以使用 IDE 的远程连接功能。

> 注意: Windows 平台上基于 WSL2 的 Docker Desktop 客户端可能在执行类似于 ``git status`` 等命令时由于文件系统效率问题可能导致卡顿。 

Ubuntu 系统可能由于语言环境问题将 ``/home/{user}`` 中的目录显示为非英文, 通过如下命令可以实现转换:

```shell
export LANG=en_US
xdg-user-dirs-gtk-update
```
