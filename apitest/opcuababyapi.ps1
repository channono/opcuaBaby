# --- API 配置 ---
$restApiUrl = "http://192.168.0.40:8080/api/v1"
# 禁用进度条显示
$ProgressPreference = 'SilentlyContinue'

# --- 函数: 读取节点 ---
function Read-Node {
    param (
        [string]$nodeId
    )
    $url = "$restApiUrl/read"
    $payload = @{
        node_id = $nodeId
    } | ConvertTo-Json

    try {
        # 发送 POST 请求，PowerShell 会自动处理 JSON
        $response = Invoke-RestMethod -Uri $url -Method Post -Body $payload -ContentType "application/json"
        Write-Host "Read Response:"
        $response | ConvertTo-Json -Depth 100 # 格式化输出
    }
    catch {
        Write-Error "Failed to read node: $_"
    }
}

# --- 函数: 写入节点 ---
function Write-Node {
    param (
        [string]$nodeId,
        [string]$dataType,
        [string]$value
    )
    $url = "$restApiUrl/write"
    $payload = @{
        node_id   = $nodeId
        data_type = $dataType
        value     = $value
    } | ConvertTo-Json

    try {
        $response = Invoke-RestMethod -Uri $url -Method Post -Body $payload -ContentType "application/json"
        Write-Host "Write Response:"
        $response | ConvertTo-Json -Depth 100
    }
    catch {
        Write-Error "Failed to write node: $_"
    }
}


# --- 调用示例 ---
Write-Host "--- REST API Demo ---" -ForegroundColor Yellow

# 读取一个节点
Read-Node -nodeId "ns=7;s=%SECOND"
Read-Node -nodeId "ns=7;s=%RAND"
Read-Node -nodeId "ns=7;s=%VA1"

# 写入一个节点
Write-Node -nodeId "ns=7;s=%VA1" -dataType "Double" -value 222.123