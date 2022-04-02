FROM ubuntu:21.10

RUN cp /etc/apt/sources.list /etc/apt/sources.list.ubuntu && \
grep -Ev '^$|^#' /etc/apt/sources.list.ubuntu > /etc/apt/sources.list

RUN sed -i 's/archive.ubuntu.com/mirrors.tuna.tsinghua.edu.cn/g' /etc/apt/sources.list && \
sed -i 's/security.ubuntu.com/mirrors.tuna.tsinghua.edu.cn/g' /etc/apt/sources.list && \
cp /etc/apt/sources.list /etc/apt/sources.list.tuna 

# Please select the geographic area in which you live. Subsequent configuration
# questions will narrow this down by presenting a list of cities, representing
# the time zones in which they are located.
#   1. Africa   3. Antarctica  5. Arctic  7. Atlantic  9. Indian    11. US
#   2. America  4. Australia   6. Asia    8. Europe    10. Pacific  12. Etc
# Geographic area:
# 修复以上问题:
ARG DEBIAN_FRONTEND=noninteractive

RUN apt update && \
apt upgrade -y && \
apt install golang build-essential cmake git vim bash-completion openssh-server curl wget tree net-tools -y && \
/etc/init.d/ssh start && \
systemctl enable ssh && \
netstat -tunlap

RUN sed -i 's/#PermitRootLogin\ prohibit-password/PermitRootLogin\ yes/g' /etc/ssh/sshd_config

RUN mkdir -p /home/remote/go/src /home/remote/go/pkg /home/remote/go/src/k8s.io

RUN useradd remote -s /bin/bash -d /home/remote -p remote

RUN go env -w GO111MODULE=on && \
go env -w GOPROXY=https://goproxy.cn,direct && \
go env -w GOPATH=/home/root/go && \
go env -w GOBIN=/home/root/go/bin

RUN su - remote

RUN go env -w GO111MODULE=on && \
go env -w GOPROXY=https://goproxy.cn,direct && \
go env -w GOPATH=/home/remote/go && \
go env -w GOBIN=/home/remote/go/bin
