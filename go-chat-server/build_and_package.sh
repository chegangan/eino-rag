#!/bin/bash

# 本地打包脚本：编译Go项目并打包为deploy.tar.gz

echo "开始编译Go项目..."
cd src/server
CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o chat-server
if [ $? -ne 0 ]; then
    echo "编译失败！"
    exit 1
fi

echo "创建部署目录..."
cd ../..
mkdir -p deploy
mkdir -p deploy/server

echo "复制文件到部署目录..."
cp src/server/chat-server deploy/server/
cp -r src/client deploy/
cp src/server/config.json deploy/server/

echo "打包文件..."
tar -czf deploy.tar.gz deploy/

echo "打包完成！文件：deploy.tar.gz"
echo "使用 scp deploy.tar.gz user@remote:/path/ 发送到远程服务器"
scp deploy.tar.gz root@srv29627.blue.kundencontroller.de:~/
