# Memory Harness Linux 无界面服务器部署

Linux 版是与桌面端共用同一套记忆引擎、SQLite 数据、HTTP 能力和 MCP 工具的无界面服务。它不需要显示器、X11、Wayland 或浏览器，适合实验室 Linux 服务器、云主机和长期在线的 Agent 主机。

## 支持范围

- Linux x64：`Memory-Harness-2.2.0-linux-x64.tar.gz`
- Linux ARM64：`Memory-Harness-2.2.0-linux-arm64.tar.gz`
- 进程管理：systemd
- 默认监听：`127.0.0.1:19777`
- 默认数据：`/var/lib/memory-harness/data`
- 默认日志：systemd journal

服务只监听服务器本机地址，不直接开放公网端口。远程访问使用 SSH 隧道；不要把记忆 API 通过防火墙直接暴露到互联网。

## 一、选择正确的包

在服务器执行：

```bash
uname -m
```

- 返回 `x86_64`：下载 `linux-x64` 包；
- 返回 `aarch64` 或 `arm64`：下载 `linux-arm64` 包。

下载 Release 中的 `SHA256SUMS-linux.txt` 后校验：

```bash
sha256sum --check SHA256SUMS-linux.txt
```

## 二、安装并常驻后台

```bash
tar -xzf Memory-Harness-2.2.0-linux-x64.tar.gz
cd Memory-Harness-2.2.0-linux-x64
sudo ./install.sh
./healthcheck.sh
```

安装脚本会：

1. 创建无登录权限的 `memory-harness` 系统用户；
2. 把程序安装到 `/usr/local/bin/memoryosd`；
3. 把数据固定在 `/var/lib/memory-harness/data`；
4. 注册并启动 `memory-harness.service`；
5. 设置为服务器开机自动启动。

查看状态和日志：

```bash
sudo systemctl status memory-harness --no-pager
sudo journalctl -u memory-harness -n 100 --no-pager
```

重启服务：

```bash
sudo systemctl restart memory-harness
```

## 三、迁移已有记忆库

先在原设备使用 `memoryosd export` 生成带校验的导出包。首次安装 Linux 服务时先不启动：

```bash
sudo ./install.sh --no-start
sudo -u memory-harness /usr/local/bin/memoryosd restore \
  --input /path/to/memory-export.tar.gz \
  --home /var/lib/memory-harness/data
sudo systemctl start memory-harness
./healthcheck.sh
```

`restore` 要求目标目录为空。如果服务已经创建了新数据，请先保留备份并选择新的空目录，不要覆盖原始 Evidence。

## 四、连接 Codex 或其他 Agent

先在桌面端“连接与健康”中创建独立 Agent，选择项目和权限，并保存只显示一次的 Token。把记忆库导出并恢复到 Linux 后，该 Agent 身份和权限会随数据保留。

如果 Codex 与 Memory Harness 在同一台 Linux 服务器上，MCP 配置使用：

```json
{
  "mcpServers": {
    "memory-harness": {
      "command": "/usr/local/bin/memoryosd",
      "args": ["mcp", "--endpoint", "http://127.0.0.1:19777"],
      "env": {
        "MEMORYOS_AGENT_TOKEN": "<请写入平台的秘密管理，不要提交到仓库>"
      }
    }
  }
}
```

Token 不要写入源码、普通说明文件、聊天记录、运行日志或 Evidence。每个 Agent 应使用独立 Token，便于撤销、轮换和审计。

## 五、从自己的电脑安全访问服务器

服务不开放公网监听。需要临时访问时，在自己的电脑执行：

```bash
ssh -N -L 19777:127.0.0.1:19777 user@your-server
```

隧道建立后，本机的 `http://127.0.0.1:19777/health` 会转到服务器。MCP 端点仍可写成 `http://127.0.0.1:19777`。

## 六、升级与卸载

升级时解压新包，再次运行：

```bash
sudo ./install.sh
```

安装脚本只替换程序与服务定义，不删除 `/var/lib/memory-harness/data` 和 `/etc/memory-harness/memory-harness.env`。

卸载程序：

```bash
sudo ./uninstall.sh
```

卸载脚本会停止服务并删除程序，但故意保留数据和配置，避免误删记忆。确认不再需要时，应由管理员单独备份并人工处理这些目录。

## 七、排查顺序

```bash
./healthcheck.sh
sudo systemctl status memory-harness --no-pager
sudo journalctl -u memory-harness -n 200 --no-pager
sudo -u memory-harness /usr/local/bin/memoryosd doctor --home /var/lib/memory-harness/data --addr 127.0.0.1:19778
```

常见情况：

- `address already in use`：19777 已被其他进程占用；
- `permission denied`：数据目录所有者不是 `memory-harness`；
- MCP 提示未授权：Agent Token 未设置、已撤销或没有目标项目权限；
- 模型调用失败：检查模型 SecretRef 和服务器出站网络；本地规则、FTS 与本地特征嵌入不需要外部模型。
