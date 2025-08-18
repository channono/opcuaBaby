# OPC UA 客户端证书自动生成功能

## 概述

本功能为 OPC UA Baby 客户端应用程序添加了在移动设备上自动创建客户端认证文件（.der 证书文件和 .pem 私钥文件）的能力。这对于移动设备特别有用，因为用户通常无法手动管理证书文件。

## 功能特性

### 🔐 自动证书生成
- **智能检测**: 自动检测是否需要生成新证书
- **移动优化**: 针对 iOS 和 Android 设备优化
- **标准兼容**: 生成符合 OPC UA 标准的 X.509 证书
- **安全存储**: 证书存储在应用程序的安全目录中

### 📱 移动设备支持
- **iOS**: 证书存储在 `Documents/certificates/` 目录
- **Android**: 证书存储在应用内部存储的 `files/certificates/` 目录
- **桌面**: 证书存储在 `~/.opcuababy/certificates/` 目录

### 🎯 用户界面集成
- **设置选项**: 在连接设置中提供"自动生成证书"选项
- **手动生成**: 提供"生成证书"按钮用于手动创建新证书
- **证书信息**: 提供"证书信息"按钮查看当前证书详情

## 技术实现

### 核心模块

#### 1. 证书生成器 (`internal/cert/generator.go`)
```go
// 自动生成证书文件
certPath, keyPath, err := cert.AutoGenerateCertificates()

// 验证证书文件
err := cert.ValidateCertificateFiles(certPath, keyPath)

// 获取证书信息
info, err := cert.GetCertificateInfo(certPath)
```

#### 2. 配置集成 (`internal/opc/config.go`)
```go
type Config struct {
    // ... 其他字段
    AutoGenerateCert bool `json:"auto_generate_cert,omitempty"`
}

// 确保证书可用
err := config.EnsureCertificates()
```

#### 3. UI 集成 (`internal/ui/ui.go`)
- 在设置对话框中添加证书管理选项
- 连接前自动检查和生成证书
- 提供手动证书管理功能

### 证书规格

生成的证书具有以下特性：

- **算法**: RSA 2048位（移动设备优化）
- **有效期**: 2年
- **用途**: 客户端认证和服务器认证
- **格式**: 
  - 证书: DER 格式 (`.der`)
  - 私钥: PEM 格式 (`.pem`)
- **主题**: 包含主机名和组织信息
- **扩展**: 包含 DNS 名称和 Application URI

### 示例证书信息
```
Subject: CN=OPC UA Client - hostname,OU=Client Applications,O=OPC UA Baby,L=San Francisco,ST=CA,C=US
Valid from: 2025-08-17 08:27:42
Valid until: 2027-08-17 08:27:42
Serial Number: 1
DNS Names: hostname, localhost
URIs: urn:hostname:opcuababy
```

## 使用方法

### 1. 启用自动生成
在连接设置中勾选"自动生成证书"选项。移动设备上默认启用。

### 2. 手动生成证书
1. 打开连接设置
2. 点击"生成证书"按钮
3. 系统将自动生成新的证书和私钥文件
4. 证书路径将自动填入相应字段

### 3. 查看证书信息
1. 在连接设置中点击"证书信息"按钮
2. 查看证书的详细信息，包括有效期、主题等

### 4. 安全连接
当使用 `SignAndEncrypt` 安全模式时：
1. 系统自动检查证书是否存在且有效
2. 如果启用了自动生成，将自动创建缺失的证书
3. 使用生成的证书建立安全连接

## 存储位置

### 移动设备
- **iOS**: `~/Documents/certificates/`
- **Android**: `~/files/certificates/`

### 桌面系统
- **所有平台**: `~/.opcuababy/certificates/`

### 文件命名
- 证书文件: `client.der`
- 私钥文件: `client.pem`

## 安全考虑

1. **私钥保护**: 私钥文件权限设置为仅当前用户可读
2. **证书验证**: 连接前验证证书和私钥的匹配性
3. **有效期检查**: 自动检测过期证书并重新生成
4. **安全存储**: 证书存储在应用程序的受保护目录中

## 故障排除

### 常见问题

1. **证书生成失败**
   - 检查应用程序是否有写入权限
   - 确保存储空间充足

2. **证书验证失败**
   - 检查证书和私钥文件是否完整
   - 验证文件权限设置

3. **连接失败**
   - 确认服务器支持客户端证书认证
   - 检查证书是否在服务器信任列表中

### 日志信息
应用程序会记录证书相关的操作日志：
- 证书生成成功/失败
- 证书验证结果
- 证书信息摘要

## API 参考

### 证书生成配置
```go
type CertificateConfig struct {
    CommonName         string
    Organization       string
    OrganizationalUnit string
    Country            string
    Province           string
    Locality           string
    ApplicationURI     string
    ValidityDays       int
    KeySize            int
    DNSNames           []string
    IPAddresses        []net.IP
}
```

### 主要函数
```go
// 获取移动设备默认配置
config := cert.MobileConfig()

// 生成证书文件
err := cert.GenerateCertificateFiles(config, certPath, keyPath)

// 获取移动存储路径
storageDir, err := cert.GetMobileStoragePath()
```

## 更新日志

- **v1.0**: 初始实现，支持自动证书生成
- 添加移动设备优化
- 集成到 UI 设置界面
- 支持证书信息查看和手动生成
