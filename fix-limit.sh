#!/usr/bin/env bash

echo 'Starting...'
echo '[1/4] 修改目录权限...'
sudo find . -type d -exec chmod 755 {} \;
echo '[2/4] 修改隐藏目录权限...'
sudo find . -type d -name '.*' -exec chmod 755 {} \;
echo '[3/4] 修改文件权限...'
sudo find . -type f -exec chmod 644 {} \;
find . -type f -name "*.sh" -exec chmod +x {} \;
echo '[4/4] 修改隐藏文件权限...'
sudo find . -type f -name '.*' -exec chmod 644 {} \;
echo 'Done'