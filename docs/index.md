# opcuaBaby Docs

An OPC UA cross‑platform desktop client written in Go. Built‑in REST API and WebSocket streaming.

- Repo: https://github.com/channono/opcuababy
- OpenAPI: ../openapi.yaml

## Install
- Download binaries from Releases (macOS/Windows/Linux) once available.
- Or build from source:

```bash
go mod download
go build ./...
```

## Quick Start
1. Run the app:
```bash
go run ./main.go
```
2. Open Settings and set Endpoint URL (e.g., `opc.tcp://host:4840`).
3. To test quickly, use Security Mode `None` + Anonymous.
4. REST base path: `http://localhost:8080/api/v1`.

### REST Examples
- Read
```bash
curl -s http://localhost:8080/api/v1/read \
  -H 'Content-Type: application/json' \
  -d '{"node_id":"ns=1;i=43335"}'
```
- Write
```bash
curl -s http://localhost:8080/api/v1/write \
  -H 'Content-Type: application/json' \
  -d '{"node_id":"ns=1;i=43335","data_type":"Int32","value":"123"}'
```

### WebSocket Subscribe
- Endpoint: `GET /ws/subscribe`
- Client messages (JSON):
```
{ "action": "subscribe", "node_ids": ["ns=1;i=43335"] }
{ "action": "unsubscribe", "node_ids": ["ns=1;i=43335"] }
{ "action": "subscribe_all" }
{ "action": "unsubscribe_all" }
```

## Security & Authentication
- Policies: None, Basic256Sha256
- Modes: None, Sign, SignAndEncrypt
- Auth: Anonymous, Username
- One‑click certificate generation (ensure server trusts generated CA for secure modes)

## FAQ
- Connection fails? Try `None + Anonymous` first, check firewall and endpoint.
- Secure mode fails? Export generated CA and add to server trust list.

<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "SoftwareApplication",
  "name": "opcuaBaby",
  "applicationCategory": "DeveloperApplication",
  "operatingSystem": "macOS, Windows, Linux",
  "softwareVersion": "0.0.1",
  "description": "Go-based OPC UA desktop client with REST API and WebSocket streaming. Simple certificates, JSON/CSV export, cross-platform.",
  "license": "https://opensource.org/licenses/MIT",
  "url": "https://github.com/channono/opcuababy",
  "downloadUrl": "https://github.com/channono/opcuababy/releases",
  "screenshot": [
    "https://raw.githubusercontent.com/channono/opcuababy/main/assets/icon.png"
  ]
}
</script>
