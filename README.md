# opcuaBaby

![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go) ![License](https://img.shields.io/badge/License-MIT-green) ![Platform](https://img.shields.io/badge/Platform-macOS%20%7C%20Windows%20%7C%20Linux-informational)

[English](#opcuaBaby) | [简体中文](#简体中文) | [日本語](#日本語)

An OPC UA cross‑platform desktop client written in Go, with a built‑in REST API and WebSocket streaming for address space browse/read/write, tag subscriptions, and JSON/CSV export. Supports Security Policies None and Basic256Sha256 with Modes None/Sign/SignAndEncrypt, Anonymous and Username authentication, plus one‑click certificate generation.

## Table of Contents
- [Features](#features)
- [Why opcuaBaby](#why-opcuababy)
- [Build](#build)
- [Run](#run)
- [Connection Settings](#connection-settings)
- [Simplified Certificates UI](#simplified-certificates-ui)
- [Security & Authentication](#security--authentication)
- [REST API](#rest-api)
- [WebSocket](#websocket)
- [Notes](#notes)
- [FAQ](#faq)
- [License](#license)
- [简体中文](#简体中文)
- [日本語](#日本語)

## Features
* __OPC UA client__: Browse address space, read/write values, watch updates.
* __REST API__: Export address space, read/write nodes via HTTP.
* __WebSocket__: Subscribe to live watch updates.
* __Config UI__: Simplified certificate section with a single Generate button.

## Why opcuaBaby
* __Built‑in APIs__: REST + WebSocket out of the box for automation and integrations.
* __Simple security__: One‑click certificate generation; easy trust workflow.
* __Fast export__: Dump variables as JSON/CSV; folder‑scoped export.
* __Live watch streaming__: Subscribe/unsubscribe at runtime.
* __Lightweight Go binary__: Cross‑platform desktop app with minimal footprint.
* __Clear defaults__: Works with Security None + Anonymous for quick testing.

## Build
Prerequisites: Go 1.21+.

```bash
go mod download
go build ./...
```

## Run
```bash
go run ./main.go
```

Then open the UI window. The embedded API server listens on the configured port (default `8080`).

## Connection Settings
Open Settings in the app to configure:
* __Endpoint URL__ (e.g., `opc.tcp://host:4840`)
* __Security__: Policy and Mode
* __Authentication__: Anonymous or Username
* __Certificates__: Client certificate and private key paths
* __API__: Port and enable/disable

### Simplified Certificates UI
Just one click Generate Certificates button. This generates and selects the local CA certificate and private key for the client security channel.  Ensure your server trusts the generated CA certificate for secure connections.

## Security & Authentication
* __Security Policies__: None, Basic256Sha256
* __Security Modes__: None, Sign, SignAndEncrypt
* __Authentication__: Anonymous, Username
* __Behavior__: When Security Mode is `None`, certificate/key fields are hidden and only Anonymous auth is available.
* __Certificates__: Use Generate to create/select the local CA cert/key, and trust the generated CA on your server for secure modes.

## REST API
Base path: `/api/v1`

* __Export all variables__
  - GET `/export/tags?format=json|csv` (default json)

* __Export variables under a folder__
  - GET `/export/tags/folder?node_id=<NodeID>&recursive=true|false&format=json|csv`

* __Read__
  - POST `/read`
  - Body:
    ```json
    { "node_id": "ns=1;i=43335" }
    ```

* __Write__
  - POST `/write`
  - Body:
    ```json
    { "node_id": "ns=1;i=43335", "data_type": "Int32", "value": "123" }
    ```

## WebSocket
Live updates for watched nodes.

* __Endpoint__: `GET /ws/subscribe`
* __Client control messages__ (JSON):
  ```json
  { "action": "subscribe", "node_ids": ["ns=1;i=43335"] }
  { "action": "unsubscribe", "node_ids": ["ns=1;i=43335"] }
  { "action": "subscribe_all" }
  { "action": "unsubscribe_all" }
  ```
* __List WS clients__: `GET /api/v1/ws/clients`

## Notes
* Default API port is `8080`. Change it in Settings.
* When Security Mode is `None`, certificate/key fields are hidden and only Anonymous auth is available.
* For secure modes, provide the certificate and key paths or use Generate to create/select the local CA cert/key.

## FAQ
* __Cannot connect to server?__
  - Try Security Mode `None` + Anonymous first to validate connectivity.
  - Verify the endpoint URL (e.g., `opc.tcp://host:4840`) and firewall rules.
* __My server requires Anonymous with Security=None__
  - Set Security Mode `None` and choose Anonymous. Certificate fields will be hidden by design.
* __Secure connection fails due to trust__
  - Export the generated CA certificate and add it to the server trust list.
* __API port already in use__
  - Change the API port in Settings or free the port.

## License
MIT

---

## 简体中文

一个带内置 REST API 与 WebSocket 实时订阅的 OPC UA 桌面客户端。

### 功能
* __OPC UA 客户端__：浏览地址空间、读/写数值、监视节点更新。
* __REST API__：通过 HTTP 导出地址空间、读/写节点。
* __WebSocket__：订阅监视列表的实时更新。
* __配置界面__：证书区域简化为一个“生成证书”按钮。

### 构建
前置条件：Go 1.21+。

```bash
go mod download
go build ./...
```

### 运行
```bash
go run ./main.go
```

随后打开应用窗口。内嵌的 API 服务器在配置的端口监听（默认 `8080`）。

### 连接设置
在应用的“设置”中配置：
* __Endpoint URL__（例如 `opc.tcp://host:4840`）
* __安全性__：策略与模式
* __认证__：匿名或用户名/密码
* __证书__：客户端证书与私钥路径
* __API__：端口与开关

#### 简化证书界面
仅需一次点击“生成证书”。这会为客户端安全通道生成并选择本地 CA 证书与私钥。若使用安全模式，请确保服务器信任生成的 CA 证书。

### REST API
基础路径：`/api/v1`

* __导出全部变量__
  - GET `/export/tags?format=json|csv`（默认 json）

* __导出指定文件夹下的变量__
  - GET `/export/tags/folder?node_id=<NodeID>&recursive=true|false&format=json|csv`

* __读取__
  - POST `/read`
  - 请求体：
    ```json
    { "node_id": "ns=1;i=43335" }
    ```

* __写入__
  - POST `/write`
  - 请求体：
    ```json
    { "node_id": "ns=1;i=43335", "data_type": "Int32", "value": "123" }
    ```

### WebSocket
用于监视节点的实时更新。

* __端点__：`GET /ws/subscribe`
* __客户端控制消息__（JSON）：
  ```json
  { "action": "subscribe", "node_ids": ["ns=1;i=43335"] }
  { "action": "unsubscribe", "node_ids": ["ns=1;i=43335"] }
  { "action": "subscribe_all" }
  { "action": "unsubscribe_all" }
  ```
* __列出 WS 客户端__：`GET /api/v1/ws/clients`

### 备注
* 默认 API 端口为 `8080`，可在设置中修改。
* 当安全模式为 `None` 时，会隐藏证书/私钥字段，仅支持匿名认证。
* 在安全模式下，请提供证书与私钥路径，或使用“生成证书”创建/选择本地 CA 证书与私钥。

### 许可证
MIT

---

## 日本語

REST API と WebSocket によるライブ更新を内蔵した OPC UA デスクトップクライアントです。

### 機能
* __OPC UA クライアント__：アドレス空間のブラウズ、値の読み書き、更新の監視。
* __REST API__：HTTP 経由でアドレス空間のエクスポート、ノードの読み書き。
* __WebSocket__：監視ノードのライブ更新を購読。
* __設定 UI__：証明書セクションは「証明書を生成」ボタンのみのシンプル設計。

### ビルド
前提条件：Go 1.21+。

```bash
go mod download
go build ./...
```

### 実行
```bash
go run ./main.go
```

その後、アプリのウィンドウを開きます。組み込み API サーバーは設定されたポート（デフォルト `8080`）で待ち受けます。

### 接続設定
アプリの「設定」で以下を構成します：
* __エンドポイント URL__（例：`opc.tcp://host:4840`）
* __セキュリティ__：ポリシーとモード
* __認証__：Anonymous または Username
* __証明書__：クライアント証明書と秘密鍵のパス
* __API__：ポートと有効/無効

#### 簡素化された証明書 UI
「証明書を生成」を 1 回クリックするだけで、クライアントのセキュリティチャネル用にローカル CA 証明書と秘密鍵を生成・選択します。セキュアモードを使う場合は、サーバーが生成された CA 証明書を信頼するよう設定してください。

### REST API
ベースパス：`/api/v1`

* __全変数をエクスポート__
  - GET `/export/tags?format=json|csv`（デフォルト json）

* __フォルダ配下の変数をエクスポート__
  - GET `/export/tags/folder?node_id=<NodeID>&recursive=true|false&format=json|csv`

* __読み取り__
  - POST `/read`
  - ボディ：
    ```json
    { "node_id": "ns=1;i=43335" }
    ```

* __書き込み__
  - POST `/write`
  - ボディ：
    ```json
    { "node_id": "ns=1;i=43335", "data_type": "Int32", "value": "123" }
    ```

### WebSocket
監視ノードのライブ更新。

* __エンドポイント__：`GET /ws/subscribe`
* __クライアント制御メッセージ__（JSON）：
  ```json
  { "action": "subscribe", "node_ids": ["ns=1;i=43335"] }
  { "action": "unsubscribe", "node_ids": ["ns=1;i=43335"] }
  { "action": "subscribe_all" }
  { "action": "unsubscribe_all" }
  ```
* __WS クライアント一覧__：`GET /api/v1/ws/clients`

### 注意
* デフォルトの API ポートは `8080`。設定で変更できます。
* セキュリティモードが `None` の場合、証明書/鍵の項目は非表示となり、Anonymous 認証のみ利用可能です。
* セキュアモードでは、証明書と秘密鍵のパスを指定するか、「証明書を生成」を使ってローカル CA 証明書/鍵を作成・選択してください。

### ライセンス
MIT
