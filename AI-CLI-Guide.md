# 🚀 AI 编程助手使用指南

本文档面向 **PIG Coder 平台**用户，帮助你快速上手：从平台获取 API Key，到配置各款 AI 编程助手 CLI 工具。

> **支持的 CLI 工具：** Claude Code、Codex CLI、Gemini CLI、OpenCode、Droid CLI

---

## 📋 目录

- [第一部分：平台操作](#第一部分平台操作)
- [第二部分：CLI 工具配置](#第二部分cli-工具配置)
  - [1. Claude Code](#1-claude-code)
  - [2. Codex CLI](#2-codex-cli)
  - [3. Gemini CLI](#3-gemini-cli)
  - [4. OpenCode](#4-opencode)
  - [5. Droid CLI](#5-droid-cli)
  - [6. CC Switch（可视化配置管理）](#6-cc-switch可视化配置管理)
- [第三部分：常用命令与故障排查](#第三部分常用命令与故障排查)

---

## 第一部分：平台操作

在使用任何 CLI 工具之前，你需要先在平台上完成以下操作。

### Step 1：注册 / 登录

1. 打开平台地址（管理员提供）
2. 点击 **注册** 创建账号，或直接 **登录**
3. 登录后进入用户仪表盘

### Step 2：获取 API Key

1. 在左侧导航栏点击 **API Keys**
2. 点击 **创建 API Key**
3. 填写名称（如 `my-claude-code`），方便区分用途
4. **选择分组**（重要）：
   - 分组决定了你能使用哪些模型和账号池
   - 常见分组如 `claude`、`openai`、`gemini` 等
   - 如果不确定选哪个，请咨询管理员
5. 点击 **确认创建**
6. **⚠️ 立即复制并保存 API Key**（格式如 `sk-xxxx...`），关闭弹窗后将无法再次查看

### Step 3：确认端点地址

不同 AI 服务使用不同的端点路径：

| 服务类型 | 端点地址 | 适用工具 |
|---------|---------|---------|
| **Anthropic (Claude)** | `https://sub2.pigcoder.com` | Claude Code、Droid (Anthropic) |
| **OpenAI (GPT/Codex)** | `https://sub2.pigcoder.com/v1` | Codex CLI、Droid (OpenAI)、OpenCode (GPT) |
| **Gemini** | `https://sub2.pigcoder.com/v1beta` | Gemini CLI、OpenCode (Gemini) |

> **⚠️ 端点路径是最常见的配置错误来源**，请务必注意 `/v1` 和 `/v1beta` 的区别。

### Step 4：查看用量

- 在左侧导航栏点击 **使用记录** 可查看详细的请求日志和 Token 消耗
- 在 **仪表盘** 可查看余额和用量概览

### Step 5：使用密钥（快速配置）

创建 API Key 后，在 Key 列表的 **操作** 栏中点击 **使用密钥** 按钮，系统会弹出该 Key 对应的配置说明，包括：

- 当前 Key 适用的 CLI 工具（Claude Code / Codex / Gemini CLI）
- 该工具所需的环境变量或配置文件内容
- 可以直接 **一键复制** 配置命令到终端执行

> 这是最快的上手方式——不需要自己拼凑端点地址和配置格式，系统已经根据你的 Key 和分组自动生成了正确的配置。

### Step 6：使用 CC Switch 一键导入（可选）

如果你已安装 [CC Switch](#6-cc-switch可视化配置管理) 桌面工具，可以在 Key 列表的 **操作** 栏中点击 **导入到 CCS** 按钮：

1. 浏览器弹出提示「要打开 CC Switch 吗？」
2. 点击 **打开 CC Switch**
3. CC Switch 自动导入 API Key、端点地址和提供商配置
4. 在 CC Switch 中点击 **Enable** 即可生效

> 这种方式完全不需要手动编辑任何配置文件，适合不熟悉命令行的用户。

---

## 第二部分：CLI 工具配置

---

### 1. Claude Code

Claude Code 是 Anthropic 官方推出的 AI 编程助手。运行依赖 **Node.js (v18+)**。

#### 🍎 macOS

**安装 Node.js：**

```bash
# 方法一：Homebrew（推荐）
brew update
brew install node

# 方法二：访问 https://nodejs.org 下载 LTS 版本

# 验证安装
node --version
npm --version
```

**安装 Claude Code（Native Install 推荐）：**

```bash
# 方法一：Homebrew
brew install --cask claude-code

# 方法二：curl 脚本
curl -fsSL https://claude.ai/install.sh | bash

# 安装最新版
curl -fsSL https://claude.ai/install.sh | bash -s latest

# 验证
claude --version
```

**连接服务（方法一：settings.json 推荐）：**

创建并编辑 `~/.claude/settings.json`：

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key-here",
    "ANTHROPIC_BASE_URL": "https://sub2.pigcoder.com",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  },
  "permissions": {
    "allow": [],
    "deny": []
  }
}
```

> 将 `your-api-key-here` 替换为你在平台创建的 API Key。

**连接服务（方法二：环境变量）：**

```bash
echo 'export ANTHROPIC_BASE_URL="https://sub2.pigcoder.com"' >> ~/.zshrc
echo 'export ANTHROPIC_AUTH_TOKEN="your-api-key-here"' >> ~/.zshrc
source ~/.zshrc
```

**VS Code 扩展：**

1. 在 VS Code 中安装 **Claude Code for VS Code** 扩展
2. 创建 `~/.claude/config.json`：
   ```json
   {
     "primaryApiKey": "any-value"
   }
   ```
3. 在项目目录下运行 `claude` 即可启动

---

#### 🪟 Windows

**安装 Node.js：**

- **方法一：** 访问 [Node.js 官网](https://nodejs.org/) 下载 LTS 版本安装（推荐）
- **方法二：** 包管理器
  ```powershell
  choco install nodejs
  # 或
  scoop install nodejs
  ```

**安装 Claude Code（Native Install 推荐）：**

使用 PowerShell 运行：

```powershell
# 安装稳定版
irm https://claude.ai/install.ps1 | iex

# 验证
claude --version
```

**连接服务（方法一：settings.json 推荐）：**

创建并编辑 `C:\Users\你的用户名\.claude\settings.json`，内容与 macOS 相同：

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key-here",
    "ANTHROPIC_BASE_URL": "https://sub2.pigcoder.com",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  },
  "permissions": {
    "allow": [],
    "deny": []
  }
}
```

**连接服务（方法二：环境变量）：**

```powershell
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_BASE_URL", "https://sub2.pigcoder.com", [System.EnvironmentVariableTarget]::User)
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_AUTH_TOKEN", "your-api-key-here", [System.EnvironmentVariableTarget]::User)
```

> 设置后需**重启 PowerShell 窗口**才能生效。

**VS Code 扩展与启动：**

1. 安装 **Claude Code for VS Code** 扩展
2. 创建 `C:\Users\你的用户名\.claude\config.json`，设置 `"primaryApiKey": "any-value"`
3. 在项目目录下运行 `claude`

---

#### 🐧 Linux

**安装 Node.js：**

```bash
# 推荐：使用 NodeSource 仓库
curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
sudo apt-get install -y nodejs

# 验证
node --version
npm --version
```

**安装 Claude Code（Native Install）：**

```bash
curl -fsSL https://claude.ai/install.sh | bash

# 验证
claude --version
```

> **Alpine Linux 用户：** 需额外安装依赖 `apk add libgcc libstdc++ ripgrep` 并设置 `export USE_BUILTIN_RIPGREP=0`。

**连接服务与启动：**

配置 `~/.claude/settings.json` 或修改 `~/.bashrc` 环境变量（同 macOS）。VS Code 扩展及启动方式与 macOS 一致。

---

### 2. Codex CLI

Codex 是 OpenAI 官方的命令行 AI 编程助手。

> **⚠️ 重要：** Codex 使用 OpenAI 兼容格式，端点必须包含 `/v1` 路径。

#### 安装（全平台通用）

确保已安装 Node.js (v18+)，然后全局安装（Windows 请以管理员身份运行）：

```bash
npm i -g @openai/codex --registry=https://registry.npmmirror.com
```

#### 连接服务（全平台通用）

**配置目录：**

| 系统 | 路径 |
|------|------|
| macOS / Linux | `~/.codex/` |
| Windows | `C:\Users\你的用户名\.codex\` |

**创建 `config.toml`：**

```toml
model_provider = "cch"
model = "gpt-5.2"
model_reasoning_effort = "xhigh"
disable_response_storage = true
sandbox_mode = "workspace-write"
# Windows 用户可添加：
# windows_wsl_setup_acknowledged = true

[features]
plan_tool = true
apply_patch_freeform = true
view_image_tool = true
web_search_request = true
unified_exec = false
streamable_shell = false
rmcp_client = true

[model_providers.cch]
name = "cch"
base_url = "https://sub2.pigcoder.com/v1"
wire_api = "responses"
requires_openai_auth = true

[sandbox_workspace_write]
network_access = true
```

**创建 `auth.json`（同目录）：**

```json
{
  "OPENAI_API_KEY": "your-api-key-here"
}
```

> 将 `your-api-key-here` 替换为你的 API Key。

**VS Code 配置：** 安装 **Codex – OpenAI's coding agent** 扩展，并设置环境变量 `CCH_API_KEY`。

**启动：** 在项目目录运行 `codex`。

---

### 3. Gemini CLI

Google 官方命令行工具，支持自动规划任务的 Agent Mode。

#### 安装（全平台通用）

确保 Node.js (v18+) 已安装：

```bash
npm install -g @google/gemini-cli
```

#### 连接服务

**配置目录：**

| 系统 | 路径 |
|------|------|
| macOS / Linux | `~/.gemini/` |
| Windows | `%USERPROFILE%\.gemini\` |

**创建 `.env` 文件：**

```env
GOOGLE_GEMINI_BASE_URL=https://sub2.pigcoder.com
GEMINI_API_KEY=your-api-key-here
GEMINI_MODEL=gemini-3-pro-preview
```

**创建 `settings.json` 文件（同目录）：**

```json
{
  "ide": { "enabled": true },
  "security": { "auth": { "selectedType": "gemini-api-key" } }
}
```

#### 启动

```bash
# 交互模式
gemini

# Agent 模式（自动任务规划）
gemini --agent
```

---

### 4. OpenCode

OpenCode 是一款终端 TUI 编程工具，可作为统一入口接入 Claude、GPT 与 Gemini。

#### 安装

```bash
# macOS / Linux
curl -fsSL https://opencode.ai/install | bash

# Windows（推荐 Chocolatey）
choco install opencode
# 或 Scoop
scoop install opencode
```

> **注意：** 不建议使用第三方 npm 镜像源安装，以免依赖缺失。

#### 连接服务（全平台通用）

**配置文件路径：**

| 系统 | 路径 |
|------|------|
| macOS / Linux | `~/.config/opencode/opencode.json` |
| Windows | `%USERPROFILE%\.config\opencode\opencode.json` |

**先设置环境变量：**

```bash
# macOS / Linux
export CCH_API_KEY="your-api-key-here"

# Windows PowerShell
[System.Environment]::SetEnvironmentVariable("CCH_API_KEY", "your-api-key-here", [System.EnvironmentVariableTarget]::User)
```

**创建 `opencode.json`：**

```json
{
  "$schema": "https://opencode.ai/config.json",
  "theme": "opencode",
  "autoupdate": false,
  "model": "openai/gpt-5.2",
  "small_model": "openai/gpt-5.2-small",
  "provider": {
    "cchClaude": {
      "npm": "@ai-sdk/anthropic",
      "name": "Claude via cch",
      "options": {
        "baseURL": "https://sub2.pigcoder.com/v1",
        "apiKey": "{env:CCH_API_KEY}"
      },
      "models": {
        "claude-sonnet-4-5-20250929": { "name": "Claude Sonnet 4.5" }
      }
    },
    "cchGPT": {
      "npm": "@ai-sdk/openai",
      "name": "GPT via cch",
      "options": {
        "baseURL": "https://sub2.pigcoder.com/v1",
        "apiKey": "{env:CCH_API_KEY}",
        "store": false,
        "setCacheKey": true
      },
      "models": {
        "gpt-5.2": {
          "name": "GPT-5.2",
          "options": { "reasoningEffort": "xhigh", "store": false }
        }
      }
    },
    "cchGemini": {
      "npm": "@ai-sdk/google",
      "name": "Gemini via cch",
      "options": {
        "baseURL": "https://sub2.pigcoder.com/v1beta",
        "apiKey": "{env:CCH_API_KEY}"
      },
      "models": {
        "gemini-3-pro-preview": { "name": "Gemini 3 Pro Preview" }
      }
    }
  }
}
```

#### 启动与切换模型

```bash
# 启动
opencode

# 在界面内输入 /models 可切换模型
```

---

### 5. Droid CLI

Factory AI 开发的交互式助手。**必须先注册并登录 Droid 官方账号。**

#### 安装

```bash
# macOS / Linux（Linux 需预装 xdg-utils）
curl -fsSL https://app.factory.ai/cli | sh

# Windows (PowerShell)
irm https://app.factory.ai/cli/windows | iex
```

#### 登录与配置

1. 运行 `droid`，按提示在浏览器完成 Factory 账号登录
2. 编辑配置文件添加自定义模型：

**配置文件路径：**

| 系统 | 路径 |
|------|------|
| macOS / Linux | `~/.factory/config.json` |
| Windows | `%USERPROFILE%\.factory\config.json` |

**添加内容：**

```json
{
  "custom_models": [
    {
      "model_display_name": "Sonnet 4.5 [cch]",
      "model": "claude-sonnet-4-5-20250929",
      "base_url": "https://sub2.pigcoder.com",
      "api_key": "your-api-key-here",
      "provider": "anthropic"
    },
    {
      "model_display_name": "GPT-5.2 [cch]",
      "model": "gpt-5.2",
      "base_url": "https://sub2.pigcoder.com/v1",
      "api_key": "your-api-key-here",
      "provider": "openai"
    }
  ]
}
```

#### 切换模型

重启 Droid，输入 `/model` 选择配置好的 cch 模型。

---

### 6. CC Switch（可视化配置管理）

**CC Switch** 是一款跨平台桌面工具，提供可视化界面来统一管理 Claude Code、Codex、Gemini CLI、OpenCode、OpenClaw 五款 CLI 工具的配置。无需手动编辑 JSON/TOML/.env 文件，一键切换 API 提供商。

> **项目地址：** [https://github.com/farion1231/cc-switch](https://github.com/farion1231/cc-switch)

#### 核心功能

- **一个应用管五个 CLI** — 统一界面管理所有 AI 编程助手配置
- **50+ 预置提供商** — 内置大量 API 提供商模板，选择后填入 Key 即可
- **一键切换** — 主界面或系统托盘快速切换，Claude Code 支持热切换无需重启
- **统一 MCP 管理** — 跨工具管理 MCP Server、Prompts、Skills
- **Deep Link 导入** — 支持 `ccswitch://` 协议，点击链接自动导入配置

#### 安装

**macOS（推荐 Homebrew）：**

```bash
brew tap farion1231/ccswitch
brew install --cask cc-switch

# 更新
brew upgrade --cask cc-switch
```

**macOS（手动下载）：**

从 [Releases](https://github.com/farion1231/cc-switch/releases) 下载 `CC-Switch-v{version}-macOS.dmg` 安装。

**Windows：**

从 [Releases](https://github.com/farion1231/cc-switch/releases) 下载：
- `CC-Switch-v{version}-Windows.msi`（安装版）
- `CC-Switch-v{version}-Windows-Portable.zip`（免安装版）

**Linux：**

从 [Releases](https://github.com/farion1231/cc-switch/releases) 下载：
- `.deb`（Debian/Ubuntu）
- `.rpm`（Fedora/RHEL）
- `.AppImage`（通用）

Arch Linux 用户：`paru -S cc-switch-bin`

#### 方式一：Deep Link 一键导入（推荐）

平台已集成 CC Switch 的 Deep Link 功能。在管理后台的 **API Key 页面**，点击 **CC Switch 导入按钮**，浏览器会弹出提示「要打开 CC Switch 吗？」，点击 **打开 CC Switch** 即可自动将以下配置导入：

- API Key
- 端点地址（自动区分 Anthropic / OpenAI / Gemini）
- 提供商名称

> 导入后在 CC Switch 中选择该提供商，点击 **Enable** 即刻生效。

#### 方式二：手动添加提供商

1. 打开 CC Switch
2. 点击 **Add Provider**
3. 选择对应的 CLI 工具（如 Claude Code）
4. 选择 **Custom**（自定义）或从预置列表中选择模板
5. 填写配置：

| 字段 | Claude Code | Codex | Gemini CLI |
|------|------------|-------|------------|
| **Name** | `PIG Coder Claude` | `PIG Coder Codex` | `PIG Coder Gemini` |
| **Base URL** | `https://sub2.pigcoder.com` | `https://sub2.pigcoder.com/v1` | `https://sub2.pigcoder.com` |
| **API Key** | 你的 API Key | 你的 API Key | 你的 API Key |

6. 点击 **Save**
7. 在主界面选中该提供商，点击 **Enable**

#### 切换与生效

- **主界面切换**：选择提供商 → 点击 **Enable**
- **系统托盘快切**：右键托盘图标 → 直接点击提供商名称
- **生效方式**：Claude Code 热切换即时生效；Codex / Gemini CLI / OpenCode 需重启终端

---

## 第三部分：常用命令与故障排查

### 常用命令

以下命令在大多数 CLI 工具中通用：

| 命令 | 功能 | 适用工具 |
|------|------|---------|
| `/help` | 查看帮助信息 | 全部 |
| `/clear` | 清空对话历史，开启新对话 | Claude Code、Codex |
| `/compact` | 总结并压缩当前对话 | Claude Code |
| `/cost` | 查看当前对话已使用的金额 | Claude Code |
| `/model` | 切换模型 | Droid、OpenCode |
| `/models` | 查看可用模型列表 | OpenCode |

### 故障排查

#### ❌ 命令未找到（Command not found）

通常是 npm 全局安装路径未加入系统 PATH。

```bash
# 查看 npm 全局路径
npm config get prefix

# 将输出路径的 bin 子目录添加到 PATH
# macOS / Linux: 添加到 ~/.zshrc 或 ~/.bashrc
# Windows: 添加到系统环境变量
```

#### ❌ API 连接失败

```bash
# 检查网络连通性
curl -I https://sub2.pigcoder.com

# Windows PowerShell
Test-NetConnection sub2.pigcoder.com -Port 443

# 检查环境变量是否正确加载
echo $ANTHROPIC_AUTH_TOKEN        # macOS / Linux
echo $env:ANTHROPIC_AUTH_TOKEN    # Windows PowerShell
```

#### ❌ 端点配置错误（最常见问题）

这是最容易犯的错误，不同协议对路径的要求不同：

| 协议格式 | 正确端点 | 适用场景 |
|---------|---------|---------|
| **Anthropic** | `https://sub2.pigcoder.com`（无 `/v1`） | Claude Code、Droid (Anthropic) |
| **OpenAI** | `https://sub2.pigcoder.com/v1`（必须 `/v1`） | Codex、Droid (OpenAI)、OpenCode (GPT) |
| **Gemini** | `https://sub2.pigcoder.com/v1beta` | Gemini CLI、OpenCode (Gemini) |

> **典型错误：** Claude Code 配了 `https://sub2.pigcoder.com/v1` → 请求失败。去掉 `/v1` 即可。

#### ❌ API Key 无效

1. 确认 Key 是否已过期或被禁用（登录平台查看）
2. 确认 Key 所属的**分组**是否包含你要使用的模型
3. 确认 Key 余额是否充足

---

## 快速对照表

| 我想用... | 安装命令 | 配置文件 | 端点地址 |
|----------|---------|---------|---------|
| **Claude Code** | `brew install --cask claude-code` | `~/.claude/settings.json` | `https://sub2.pigcoder.com` |
| **Codex CLI** | `npm i -g @openai/codex` | `~/.codex/config.toml` + `auth.json` | `https://sub2.pigcoder.com/v1` |
| **Gemini CLI** | `npm i -g @google/gemini-cli` | `~/.gemini/.env` + `settings.json` | `https://sub2.pigcoder.com` |
| **OpenCode** | `curl -fsSL https://opencode.ai/install \| bash` | `~/.config/opencode/opencode.json` | 按模型区分 |
| **Droid CLI** | `curl -fsSL https://app.factory.ai/cli \| sh` | `~/.factory/config.json` | 按 provider 区分 |
| **CC Switch** | `brew install --cask cc-switch` | 可视化界面管理 | 自动配置 |