# opcuaBaby

An OPC UA desktop client with a built‑in REST API and WebSocket streaming for tag updates.

## Features
* __OPC UA client__: Browse address space, read/write values, watch updates.
* __REST API__: Export address space, read/write nodes via HTTP.
* __WebSocket__: Subscribe to live watch updates.
* __Config UI__: Simplified certificate section with a single Generate button.

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
just one click Generate Certificates button. This generates and selects the local CA certificate and private key for the client security channel.  Ensure your server trusts the generated CA certificate for secure connections.

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

## License
MIT
