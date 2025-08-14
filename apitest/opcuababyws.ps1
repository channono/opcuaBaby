# --- API 配置 ---
$wsApiUrl = "ws://192.168.0.40:8080/ws/subscribe"

# 禁用所有进度显示
$Global:ProgressPreference = 'SilentlyContinue'
$ErrorActionPreference = 'Stop'  # 让错误更清晰
$nodeIdsToSubscribe = @("ns=7;s=%SECOND", "ns=7;s=%RAND" ) # 要订阅的节点列表

# --- WebSocket 实现 ---
$ws = New-Object System.Net.WebSockets.ClientWebSocket
$cancellationTokenSource = New-Object System.Threading.CancellationTokenSource

Write-Host "Connecting to WebSocket server at $wsApiUrl..." -ForegroundColor Yellow

try {
    # 1. 连接到服务器
    $connectTask = $ws.ConnectAsync($wsApiUrl, $cancellationTokenSource.Token)
    $connectTask.Wait() # 等待连接完成
    Write-Host "WebSocket Connected." -ForegroundColor Green

    # 2. 发送订阅消息
    $subscribeMessage = @{
        action   = "subscribe"
        node_ids = $nodeIdsToSubscribe
    } | ConvertTo-Json
    
    $bytesToSend = [System.Text.Encoding]::UTF8.GetBytes($subscribeMessage)
    $bufferToSend = [System.ArraySegment[byte]]::new($bytesToSend)
    
    $sendTask = $ws.SendAsync($bufferToSend, "Text", $true, $cancellationTokenSource.Token)
    $sendTask.Wait()
    Write-Host "Subscription message sent for: $($nodeIdsToSubscribe -join ', ')"

    # 3. 循环接收消息
    Write-Host "Listening for messages... Press 'q' to quit."
    $receiveBuffer = [System.ArraySegment[byte]]::new((New-Object byte[] 8192))
    
    while ($ws.State -eq 'Open') {
        # 检查是否有按键，以便退出
        if ($Host.UI.RawUI.KeyAvailable -and ($Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown").Character -eq 'q')) {
            break
        }

        $receiveTask = $ws.ReceiveAsync($receiveBuffer, $cancellationTokenSource.Token)
        # 等待一小段时间，避免 CPU 占用过高
        $null = $receiveTask.Wait(100)  # 使用 $null = 来忽略返回值

        if ($receiveTask.IsCompleted) {
            $result = $receiveTask.Result
            if ($result.MessageType -eq 'Close') {
                Write-Warning "Server initiated close. Status: $($result.CloseStatusDescription)"
                break
            } else {
                $receivedString = [System.Text.Encoding]::UTF8.GetString($receiveBuffer.Array, 0, $result.Count)
                Write-Host "Message Received: $receivedString" -ForegroundColor Cyan
            }
        }
    }
}
catch {
    Write-Error "An error occurred: $_"
}
finally {
    # 4. 关闭连接
    if ($ws.State -eq 'Open') {
        Write-Host "Closing connection..."
        $ws.CloseAsync('NormalClosure', 'Client closing', $cancellationTokenSource.Token).Wait()
    }
    $ws.Dispose()
    $cancellationTokenSource.Dispose()
    Write-Host "WebSocket disconnected." -ForegroundColor Yellow
}