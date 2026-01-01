# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]
### Added
- 🔐 Full support for all modern OPC UA AES encryption security policies:
  - `Aes128_Sha256_RsaOaep` - AES-128 with SHA256 and RSA-OAEP
  - `Aes128_Sha256_RsaPss` - AES-128 with SHA256 and RSA-PSS
  - `Aes256_Sha256_RsaOaep` - AES-256 with SHA256 and RSA-OAEP
  - `Aes256_Sha256_RsaPss` - AES-256 with SHA256 and RSA-PSS (Highest security)
- UI security policy dropdown now includes all 9 security policy options
- Comprehensive security policy documentation in `docs/SECURITY_POLICIES.md`
- Support for case-insensitive policy name variants

### Changed
- Enhanced security policy configuration with detailed comments
- Organized security policies into categories (Deprecated, Recommended, Modern AES)

## [v0.0.1] - 2025-08-22
### Added
- Initial public release of opcuaBaby.
- Built-in REST API (`/api/v1`) and WebSocket streaming (`/ws/subscribe`).
- Read/Write nodes, export variables (JSON/CSV), list WebSocket clients.
- Simplified certificate UI with one-click generation.
- Security Policies: None, Basic256Sha256; Modes: None, Sign, SignAndEncrypt.
- Authentication: Anonymous, Username.

### Docs
- README with Features, Security & Authentication, REST & WS examples.
- OpenAPI specification at `openapi.yaml`.

[Unreleased]: https://github.com/channono/opcuababy
[v0.0.1]: https://github.com/channono/opcuababy/releases/tag/v0.0.1
