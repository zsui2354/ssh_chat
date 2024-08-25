# Devzat 管理员手册

本文档适用于希望管理自托管 Devzat 服务器的用户。

随意制作一个 [new issue](https://github.com/quackduck/devzat/issues) 如果有什么东西不管用。

## Installation
```shell
git clone https://github.com/quackduck/devzat
cd devzat
```
要编译 Devzat，您需要安装最低版本为 1.17 的 Go。

现在运行 'go install' 来全局安装 Devzat 二进制文件，或者运行 'go build' 来构建二进制文件并将其保存在工作目录中。

您可能需要使用 'ssh-keygen' 命令为您的服务器生成新的密钥对。出现提示时，另存为 'devzat-sshkey'，因为这是默认位置（可以在配置中更改）。
虽然您可以使用与用户账户相同的密钥对，但建议使用新的密钥对。

## 用法

```shell
./devzat # use without "./" for a global binary
```

默认情况下，Devzat 在端口 2221 上侦听新的 SSH 连接。用户现在可以使用 `ssh -p 2221 <server-hostname>`.

将环境变量 'PORT' 设置为不同的端口号，或编辑您的配置以更改 Devzat 侦听 SSH 连接的端口。然后，用户将运行 'ssh -p <port> <server-hostname>' 加入。

## 配置

如果找不到默认配置文件，Devzat 会写入默认配置文件，因此在使用 Devzat 之前无需创建配置文件。

Devzat 在当前目录中查找配置文件的默认位置是 'devzat.yml' 。或者，它使用 'DEVZAT_CONFIG' 环境变量中设置的路径。

An example config file:示例配置文件：
```yaml
# what port to host a server on ($PORT overrides this)   在哪个端口上托管服务器（$PORT 会覆盖此设置）
port: 2221
# an alternate port to avoid firewalls                   避免防火墙的备用端口
alt_port: 443
# what port to host profiling on (unimportant)           在哪个端口上托管分析 （不重要）
profile_port: 5555
# where to store data such as bans and logs
data_dir: devzat-data
# where the SSH private key is stored
key_file: devzat-sshkey
# where an integration config is stored (optional)
integration_config: devzat-integrations.yml
# whether to censor messages (optional)
censor: true
# a list of admin IDs and notes about them   管理员 ID 列表和有关它们的注释
admins:
  d6acd2f5c5a8ef95563883032ef0b7c0239129b2d3672f964e5711b5016e05f5: 'Arkaeriit: github.com/Arkaeriit'
  ff7d1586cdecb9fbd9fcd4c9548522493c29172bc3121d746c83b28993bd723e: 'Ishan Goel: quackduck'
```

### 使用管理员权限

作为管理员，您可以禁止、取消禁止和踢出用户。登录聊天后，您可以运行如下命令：
```shell
ban <user>
ban <user> 1h10m
unban <user ID or IP>
kick <user>
```

如果运行这些命令使 Devbot 抱怨授权，您需要在配置文件的 'admins' 键下添加您的 ID（默认为 'devzat-config.yml）。

### 启用用户白名单

Devzat 可以用作私人聊天室。将以下内容添加到您的配置中：

```yaml
private: true # enable allowlist checking
allowlist: 
  272b326d7d5e9a6b1d98a10b453bdc8cc950fc15cae2c2e858e30645c72ae7c0: 'John Doe'
  ...
```

“允许列表”的格式与“管理员”列表的格式相同。添加允许的用户的 ID 和有关该用户的信息（这是为了在编辑配置文件时更容易识别 ID，并且 Devzat 不会使用它）

允许所有管理员，即使他们的 ID 不在允许列表中。因此，如果私人服务器上的每个人都是管理员，则不需要白名单，只需启用私人模式即可。

在私聊中，“#main”上的消息积压处于禁用状态。只有与您同时登录的人才能阅读您的消息。

### 启用集成

Devzat 包含自托管实例可能不需要的功能。这些称为集成。

您可以通过将配置文件中的 'integration_config' 设置为某个路径来启用这些集成：

```yaml
integration_config: devzat-integrations.yml
```
现在在该路径处创建一个新文件。这是您的集成配置文件。

#### 使用 Slack 集成

Devzat 支持通往 Slack 的桥梁。您需要一个 Slack 机器人令牌，以便 Devzat 可以向 Slack 发布消息并从 Slack 接收消息。按照指南 [here]（https://api.slack.com/authentication/basics） 获取您的令牌并添加a Slack app to your workspace. 确保它具有读取和写入范围。

将您的机器人令牌添加到您的集成配置文件中。'prefix' 键定义在 Devzat 中呈现的 Slack 消息的前缀。在 Slack 中右键单击要桥接到的频道的频道 ID，找到该频道的频道 ID。

```yaml
slack:
    token: xoxb-XXXXXXXXXX-XXXXXXXXXXXX-XXXXXXXXXXXXXXXXXXXXXXXX
    channel_id: XXXXXXXXXXX # usually starts with a C, but could be a G or D
    prefix: Slack
```

#### 使用 Discord 集成

Devzat 支持通往 Discord 的桥梁。您需要一个 Discord 机器人令牌，以便 Devzat 可以在 Discord 上发帖和接收来自该消息的消息。按照指南 [here]（https://www.writebots.com/discord-bot-token） 设置您的机器人，并确保它具有“发送消息”、“阅读消息历史记录”和“管理 Webhooks”权限。

将您的机器人令牌添加到您的集成配置文件中。'prefix' 键定义了在 Devzat 中呈现的 Discord 消息的前缀。右键单击要桥接到的通道的通道 ID。

```yaml
discord:
    token: XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
    channel_id: XXXXXXXXXXXXXXXXXXX
    prefix: Discord
    compact_mode: true # optional: disables avatars so messages take up less vertical space
```

#### 使用 Twitter 集成
Devzat 支持在 Twitter 上发布有关谁在线的更新。首先通过 [Twitter 开发者帐户]（https://developer.twitter.com/en/apply/user） 创建一个新应用程序（注意：Twitter 的 API 现在是付费的）。

现在，将相关键添加到您的集成配置文件中：
```yaml
twitter:
    consumer_key: XXXXXXXXXXXXXXXXXXXXXXXXX
    consumer_secret: XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
    access_token: XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
    access_token_secret: XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
```

### 使用插件 API 集成

Devzat 包括一个内置的 gRPC 插件 API。这对于构建您自己的集成或使用第三方集成非常有用。

有关使用 gRPC API 的文档，请参见 [此处](plugin/README.md)。此集成将 API 令牌存储在数据目录中。

```yaml
rpc:
    port: 5556 # port to listen on for gRPC clients
```

使用 API 中的 [plugin documentation](plugin/README.md) 以允许客户端进行身份验证。

您还可以对可用作身份验证令牌的密钥进行硬编码，但不建议这样做。对于试图使某些 API 客户端始终在线的服务器所有者来说，此选项可能很有用，因为管理员无法撤销此密钥（与令牌不同）。

```yaml
    key: "some string" # a string that gRPC clients authenticate with (optional)
```

您可以同时使用任意数量的集成。

您可以设置 4 个环境变量以在命令行上快速禁用集成：
* `DEVZAT_OFFLINE_TWITTER=true` will disable Twitter
* `DEVZAT_OFFLINE_SLACK=true` will disable Slack
* `DEVZAT_OFFLINE_RPC=true` will disable the gRPC server
* `DEVZAT_OFFLINE=true` will disable all integrations.

