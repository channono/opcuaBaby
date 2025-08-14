import requests
import json
import threading
import time
import websocket
import os

REST_API_URL = "http://192.168.0.40:8080/api/v1"
WS_API_URL = "ws://192.168.0.40:8080/ws/subscribe"

# 指定tag列表json文件路径
TAG_JSON_PATH = "tags.json"

def read_node(node_id):
    url = f"{REST_API_URL}/read"
    payload = {"node_id": node_id}
    headers = {'Content-Type': 'application/json'}
    try:
        response = requests.post(url, data=json.dumps(payload), headers=headers, timeout=3)
        print("Read Response:", response.json())
    except Exception as e:
        print(f"Read node failed: {e}")

def write_node(node_id, data_type, value):
    url = f"{REST_API_URL}/write"
    payload = {
        "node_id": node_id,
        "data_type": data_type,
        "value": value
    }
    headers = {'Content-Type': 'application/json'}
    try:
        response = requests.post(url, data=json.dumps(payload), headers=headers, timeout=3)
        print("Write Response:", response.json())
    except Exception as e:
        print(f"Write node failed: {e}")

def on_message(ws, message):
    print("WebSocket Message:", json.loads(message))

def on_error(ws, error):
    print("WebSocket Error:", error)

def on_close(ws, close_status_code, close_msg):
    print("WebSocket Closed")

def extract_variable_nodeids(node, result=None):
    if result is None:
        result = []
    if isinstance(node, dict):
        if node.get("nodeClass") == "NodeClassVariable" and "nodeId" in node:
            result.append(node["nodeId"])
        # 递归 children
        children = node.get("children")
        if isinstance(children, list):
            for child in children:
                extract_variable_nodeids(child, result)
    elif isinstance(node, list):
        for item in node:
            extract_variable_nodeids(item, result)
    return result

def load_tags_from_json(json_path):
    if not os.path.exists(json_path):
        print(f"Tag json file not found: {json_path}")
        return []
    with open(json_path, "r") as f:
        data = json.load(f)
    # 如果是树结构，递归提取所有变量节点
    node_ids = extract_variable_nodeids(data)
    if node_ids:
        return node_ids
    # 兼容原有格式
    if isinstance(data, list):
        return data
    elif isinstance(data, dict):
        return data.get("tags") or data.get("node_ids") or []
    return []

def on_open(ws):
    print("WebSocket Opened")
    node_ids = load_tags_from_json(TAG_JSON_PATH)
    if not node_ids:
        print("No tags loaded from json, not subscribing.")
        subscribe_msg = {"action": "subscribe"}
    else:
        print(f"Subscribing to {len(node_ids)} tags from json file.")
        subscribe_msg = {
            "action": "subscribe",
            "node_ids": node_ids
        }
    ws.send(json.dumps(subscribe_msg))

def run_websocket():
    while True:
        ws = websocket.WebSocketApp(
            WS_API_URL,
            on_open=on_open,
            on_message=on_message,
            on_error=on_error,
            on_close=on_close
        )
        print("Starting WebSocket connection...")
        ws.run_forever()
        print("WebSocket closed. Reconnecting in 5 seconds...")
        time.sleep(5)

if __name__ == "__main__":
    # REST API demo
    print("--- REST API Demo ---")
    read_node("ns=1;i=43920")
    #write_node("ns=1;s=myByteString", "String", "Hello, World!")

    # WebSocket demo (run in background thread)
    print("--- WebSocket Demo ---")
    ws_thread = threading.Thread(target=run_websocket)
    ws_thread.daemon = True
    ws_thread.start()
    print("Press Ctrl+C to exit.")
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print("Exiting...")
