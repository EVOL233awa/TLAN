# TLAN

**[English Documentation](README_EN.md)**

**TLAN** 是一款面向好友联机、远程协作与内网服务共享的自托管私有虚拟局域网（Virtual LAN）软件。  
包含 **Linux 服务端（控制面 + 中继 + STUN + 管理面板 + FRP 托管）** 与 **Windows 桌面客户端（WireGuard 虚拟网卡 + 自动 P2P / 中继切换 + FRP 端口映射）**。

## 核心特性

- **私有虚拟局域网**：基于 WireGuard + Wintun 构建二/三层加密隧道，自动分配虚拟局域网 IP，支持设备互访与多人联机。
- **P2P 优先 + 自动中继兜底**：内置 UDP STUN 打洞与服务测速；直连不可达时自动切换至服务端 TLS TCP 中继，保障稳定联通。
- **自定义用户名**：首次加入时支持自定义玩家/设备显示名称，并在已加入网络后支持随时改名并实时同步到控制面与所有成员。
- **中英双语桌面客户端**：内置 **跟随系统 / 简体中文 / English** 一键切换，支持浅色、深色及跟随系统主题、系统托盘常驻、连接诊断包导出。
- **内置端口映射（FRP）**：在客户端即可选择本地监听进程并一键发布 TCP/UDP 端口映射，配合服务端 FRPS 支持权限与端口范围管控。
- **开箱即用的一键部署与管理**：提供 Linux 一键安装脚本 `install.sh`，自动补齐运行环境与依赖、引导配置管理员密钥、配置 `systemctl` 守护进程与防火墙，并内置 Web 管理控制台（`/admin`）及 Prometheus 指标（`/metrics`）。

---

## 快速开始：Linux 一键安装服务端

支持 **Debian / Ubuntu / CentOS / RHEL / Rocky Linux / AlmaLinux / Fedora**（需 `root` 权限、`systemd` 与 `x86_64` 或 `arm64` 架构）。

### 方式一：在源码目录执行（推荐）

```bash
git clone <你的仓库地址> TLAN
cd TLAN
sudo bash install.sh
```

脚本会自动完成以下工作：
1. 检查操作系统、架构与 `systemd` 运行环境（遇到不支持的系统、非 `root` 或非 `systemd` 容器会清晰报错退出）。
2. 自动安装缺失的基础工具（`curl`、`tar`、`gzip`、`openssl`、`ca-certificates`、`libcap` 等）以及 Go（>= 1.23.1，如本机缺失或版本过低）。
3. 交互式引导输入安装参数：
   - **管理员密钥 (`Admin Token`)**：用于登录 Web 管理后台与客户端管理员模式（必填；直接回车可自动生成强随机密钥）。
   - **成员入网密钥 (`Join Code`)**：普通成员加入虚拟局域网时填写的密码（可直接回车随机生成）。
   - **公网 IP 或域名**、**虚拟网络名称**（默认 `TLAN`）与 **虚拟网段**（默认 `10.240.0.0/24`）。
4. 编译安装 `/opt/tlan/tlan-server`，下载并配置 Caddy HTTPS 网关，生成权限为 `0600` 的 `/var/lib/tlan/tlan-config.yml`。
5. 自动创建最小权限系统账号、配置并启动 `tlan.service` 与 `tlan-https.service`（`systemctl`），自动放行 `ufw` / `firewalld` 端口并校验 `/health`。

### 方式二：非交互 / 自动化参数安装

```bash
sudo bash install.sh \
  --admin-token "请替换为你的强管理员密钥" \
  --join-code "请替换为成员入网密钥" \
  --host "你的公网IP或域名" \
  --network-name "TLAN" \
  --cidr "10.240.0.0/24" \
  --non-interactive
```

更多参数（`--no-caddy`、`--binary`、`--https-port`、`--stun-port`、`--skip-firewall`、`--reconfigure`、`--check`、`--uninstall`）：

```bash
bash install.sh --help
```

### 需要放行的防火墙 / 云安全组端口

| 协议 / 端口 | 用途 | 备注 |
| --- | --- | --- |
| `TCP 8443` | 控制面与中继 HTTPS 入口（Caddy） | 客户端默认连接端口 |
| `TCP 8444` | Web 管理控制台 HTTPS 入口（Caddy） | 访问 `https://<服务器地址>:8444/admin` |
| `TCP 80` | 域名模式 Let's Encrypt 证书申请/续期 | 使用域名 + Caddy 时需要 |
| `UDP 3478` | STUN UDP 探测与 NAT 穿透 | 用于测速与 P2P 直连打洞 |

> **提示**：如果你使用**纯公网 IP**（无域名）并启用 Caddy HTTPS，Caddy 会使用内置 CA 签发证书。请从服务器导出根证书并导入 Windows 客户端的「受信任的根证书颁发机构」：
> `/var/lib/tlan-caddy/.local/share/caddy/pki/authorities/local/root.crt`

### 服务端常用运维命令

```bash
# 查看运行状态
sudo systemctl status tlan tlan-https

# 查看实时日志
sudo journalctl -u tlan -f

# 查看或修改配置文件（包含管理员密钥与入网密钥）
sudo cat /var/lib/tlan/tlan-config.yml

# 修改配置后重启生效
sudo systemctl restart tlan
```

---

## 构建与使用 Windows 客户端

### 1. 构建环境准备

- Windows 10/11 (`amd64`)
- Go >= 1.23.1
- `go-winres`（用于嵌入图标、UAC Manifest 与版本信息）：
  ```powershell
  go install github.com/tc-hib/go-winres@latest
  ```

### 2. 编译打包客户端

在项目根目录执行：

```powershell
# 生成通用版本（由用户在客户端中填写服务器地址）
.\build-windows-client.ps1

# 或在编译时预置默认服务器与 STUN 地址
.\build-windows-client.ps1 -ServerURL "https://your-server.example.com:8443" -STUNAddress "your-server.example.com:3478"
```

构建产物位于：
- 单文件 Windows 安装包（推荐）：`dist/TLAN-Setup-v1.2.0-windows-amd64.exe`
  - 支持自定义安装目录，自动将 `TLAN.exe` 与 `wintun.dll`、`frpc.exe` 整合安装
  - 自动创建开始菜单快捷方式（出现在 Windows 开始菜单 **最近添加** 与 **所有应用列表**）及系统 **应用和功能** 卸载项（`TLAN-Uninstall.exe`）
  - 安装过程可直接勾选 **开机自动启动**，与客户端设置保持同步
- 绿色免安装目录：`dist/windows/TLAN.exe`（附带 `wintun.dll`、`frpc.exe`、`tlan-icon.png`）
- 绿色压缩发布包：`dist/TLAN-client-v1.2.0-windows-amd64.zip`

> **单实例机制**：当 TLAN 已在后台或托盘运行时，再次双击启动 `TLAN.exe` 会自动唤起并前置已有窗口，不会重复打开多个实例。

---

## 版本管理与兼容校验规则（v1.2.0）

TLAN 采用 **`大版本.中版本.小版本`（`Major.Minor.Patch`）** 三段式版本号规范：

1. **小版本（`Patch`）**：仅针对客户端进行同中大版本修复、界面或体验优化，且未修改服务端通信/逻辑时递增（例如 `1.1.0` -> `1.1.1`）。
2. **中版本（`Minor`）**：只要修改了服务端功能或接口行为，即递增中版本号（例如 `1.0.0` -> `1.1.0`）。
3. **大版本（`Major`）**：发生重大架构重构或跨代更新时递增大版本号（例如 `1.x.y` -> `2.0.0`）。

**强制版本对齐机制**：
- 客户端每次请求会在请求头携带 `X-Client-Version`，服务端在所有响应头返回 `X-Server-Version` 并在登录（`/api/v1/enroll`）及成员会话请求阶段校验双方版本号。
- **只有客户端与服务端的大版本号和中版本号（`Major.Minor`）完全一致时才允许登录与连接**（例如 `v1.1.x` 客户端仅可登录 `1.1.y` 服务端；`v1.0.x` 与 `1.1.x` 之间将直接拦截登录并提示升级）。

### 3. 客户端加入与使用

1. 以管理员权限运行 `TLAN.exe`（用于创建 Wintun 虚拟网卡）。
2. 在登录页填写：
   - **用户名**（可选，留空将使用设备名，加入后也可在主界面或设置中随时修改）
   - **服务器地址**（例如 `your-server.example.com:8443` 或 `https://1.2.3.4:8443`）
   - **入网密钥**（填 `join_code` 作为普通成员加入，或填 `admin_token` 开启管理模式）
3. 点击右上角 `中 / EN` 可随时切换中英文界面，点击主题按钮可切换浅色 / 深色外观。
4. 点击主界面大按钮即可连接虚拟局域网，点击虚拟 IP 可一键复制发给好友联机。

---

## 目录结构

```text
├── cmd/
│   ├── client/          # Windows 桌面客户端（Walk GUI、WireGuard、STUN、FRP 管理）
│   ├── server/          # Linux 服务端入口（tlan-server）
│   └── install-helper/  # Linux 安装辅助检查工具
├── core/                # 核心数据模型与验证逻辑
├── internal/
│   ├── clientapi/       # 控制面与管理面 HTTP 客户端
│   ├── clientidentity/  # 本机密钥、身份与客户端设置持久化
│   ├── clientui/        # 双语错误解释与敏感信息脱敏
│   ├── config/          # tlan-config.yml 生成、校验与热加载
│   ├── control/         # 控制面 API、Web 管理后台与 Prometheus /metrics
│   ├── frp/ & frpauth/  # FRP 托管与设备级租约控制
│   ├── mesh/            # WireGuard 隧道、P2P/中继选路与保活
│   ├── network/         # STUN 探测与 TLS 中继转发
│   ├── platform/        # Windows Wintun / Linux TAP 适配与防火墙工具
│   └── storage/         # 成员、网段与状态存储
├── deploy/              # Caddyfile、systemd 单元、FRPS 部署模板
├── docs/                # Linux 部署与 FRPS 参考文档
└── install.sh           # Linux 一键安装脚本
```

## 开源与安全说明

- 本仓库不包含任何硬编码公网 IP、域名或真实生产密钥；服务端首次生成配置或运行 `install.sh` 时会自动生成独立随机密钥。
- 请勿将包含 `security.admin_token` 或 `network.join_code` 的 `tlan-config.yml` 提交到版本控制。
- 第三方依赖许可说明请参阅 [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md)。
