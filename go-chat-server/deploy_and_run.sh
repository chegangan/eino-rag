#!/bin/bash

# 远程服务器部署脚本：解压并后台运行服务

echo "解压部署包..."
tar -xzf deploy.tar.gz
if [ $? -ne 0 ]; then
    echo "解压失败！"
    exit 1
fi

echo "进入部署目录..."
cd deploy
chmod +x server/chat-server

echo "启动服务（后台运行）..."
nohup ./server/chat-server > server.log 2>&1 &
echo "服务已启动，PID: $!"
echo "日志文件：server.log"
echo "服务运行在端口8080"
