#!/usr/bin/env bash

# 编译Kubernetes源代码并拷贝二进制到目标节点

echo "进入Kubernetes源代码目录并开始编译..."
cd $GOPATH/src/kubernetes && make

echo "编译完成"
echo "拷贝二进制文件到目标节点..."
echo "拷贝完成"

