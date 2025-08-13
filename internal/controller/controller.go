// startWatchUpdatePump periodically emits the entire watch list to the UI callback,
// reducing UI refresh frequency under heavy data change load.
package controller

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"net/http"
	"opcuababy/internal/opc"
	"sort"
	"strconv"
	"strings"

	"sync"
	"time"
	"github.com/gopcua/opcua/ua"
)

// startWatchUpdatePump periodically emits the entire watch list to the UI callback,
// reducing UI refresh frequency under heavy data change load.
func (c *Controller) startWatchUpdatePump() {
    c.mu.RLock()
    ctx := c.clientCtx
    c.mu.RUnlock()
    if ctx == nil {
        return
    }
    ticker := time.NewTicker(33 * time.Millisecond)
    go func() {
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                c.mu.RLock()
                if !c.isConnected || c.OnWatchListUpdate == nil {
                    c.mu.RUnlock()
                    continue
                }
                items := make([]*WatchItem, 0, len(c.watchItems))
                for _, wi := range c.watchItems {
                    items = append(items, wi)
                }
                sort.Slice(items, func(i, j int) bool { return items[i].NodeID < items[j].NodeID })
                cb := c.OnWatchListUpdate
                c.mu.RUnlock()
                // Emit outside lock
                cb(items)
            }
        }
    }()
}
// NodeManager defines the interface for API server interactions, breaking import cycles.
type NodeManager interface {
	ReadNodeAttributes(nodeID string) (*NodeAttributes, error)
	WriteValue(nodeID, dataType, valueStr string)
	AddWatch(nodeID string)
	GetApiBroadcastChan() chan *WatchItem
	GetClientContext() context.Context
	IsLogDisabled() bool
	CollectVariableNodes(parentID string, recursive bool) ([]*ExportTag, error)
}

// ApiServerStarter defines the function signature for starting the API server.
type ApiServerStarter func(ctx context.Context, nodeMgr NodeManager, apiStatus *string, cfg *opc.Config) *http.Server

// WatchItem 监视的变量节点封装
type WatchItem struct {
	NodeID           string
	Name             string
	DataType         string
	Value            string
	Timestamp        string
	Severity         string
	SymbolicName     string
	SubCode          uint16
	StructureChanged bool
	SemanticsChanged bool
	InfoBits         uint16
	RawCode          string

	subHandle *opc.Subscription
}

// AddressSpaceNode 地址空间节点结构
type AddressSpaceNode struct {
	NodeID      string
	Name        string
	NodeClass   ua.NodeClass
	HasChildren bool
}

// NodeAttributes 节点详细属性
type NodeAttributes struct {
	NodeID      string
	Name        string
	Description string
	NodeClass   string
	DataType    string
	AccessLevel string
	Value       string
	ValueRank   int // -1: scalar; 0 or >0: array (0 = any dims, >0 = number of dimensions)
}

// ExportTag represents a tag for export
type ExportTag struct {
    NodeID      string `json:"node_id"`
    Name        string `json:"name"`
    DataType    string `json:"data_type,omitempty"`
    Description string `json:"description,omitempty"`
    Path        string `json:"path,omitempty"`
}

type Controller struct {
	client               *opc.Client
	clientLifecycleMutex sync.Mutex
	clientCtx            context.Context    // 【添加】与 client 生命周期绑定的 context
	clientCancel         context.CancelFunc // 【添加】用于取消 clientCtx 的函数

	mu           sync.RWMutex
	isConnecting bool
	isConnected  bool

	watchItems map[string]*WatchItem

	addressSpaceMutex    sync.RWMutex
	addressSpaceNodes    map[string]*AddressSpaceNode
	addressSpaceChildren map[string][]string

	browsingNodes    map[string]bool // 浏览防护，防止重复浏览
	noChildrenCached map[string]bool // 日志限流用

	logMu          sync.Mutex
	logCount       int
	logWindowStart time.Time

	// API Server fields
	apiServer       *http.Server
	apiServerCtx    context.Context
	apiServerCancel context.CancelFunc
	apiStatus       *string
	currentConfig   *opc.Config
	apiStarter      ApiServerStarter

	OnConnectionStateChange func(connected bool, endpoint string, err error)

    // UI callbacks
    OnAddressSpaceReset    func()
    OnWatchListUpdate      func(items []*WatchItem)
    OnNodeAttributesUpdate func(attrs *NodeAttributes)

    // Channels
    AddressSpaceUpdateChan chan string
    ApiBroadcastChan       chan *WatchItem
    LogChan                chan string
}

func New() *Controller {
    return &Controller{
        watchItems:            make(map[string]*WatchItem),
        addressSpaceNodes:     make(map[string]*AddressSpaceNode),
        addressSpaceChildren:  make(map[string][]string),
        browsingNodes:         make(map[string]bool),
        noChildrenCached:      make(map[string]bool),
        AddressSpaceUpdateChan: make(chan string, 64),
        ApiBroadcastChan:      make(chan *WatchItem, 64),
        LogChan:               make(chan string, 256),
    }
}

func (c *Controller) Log(msg string) {
    // Respect DisableLog when configured
    if c.currentConfig != nil && c.currentConfig.DisableLog {
        return
    }
    c.logMu.Lock()
    defer c.logMu.Unlock()
    select {
    case c.LogChan <- msg:
    default:
    }
}

// IsLogDisabled reports whether logs should be suppressed based on current config
func (c *Controller) IsLogDisabled() bool {
    c.mu.RLock()
    cfg := c.currentConfig
    c.mu.RUnlock()
    return cfg != nil && cfg.DisableLog
}

func (c *Controller) Connect(cfg *opc.Config) error {
    c.mu.Lock()
    if c.isConnected || c.isConnecting {
        c.mu.Unlock()
        c.Log("[yellow]Connect skipped: already connected or connecting[-]")
        return nil
    }
    c.isConnecting = true
    c.mu.Unlock()
    c.Log(fmt.Sprintf("[cyan]Connecting to %s...[-]", cfg.EndpointURL))

    // Create lifecycle context
    c.clientLifecycleMutex.Lock()
    if c.clientCancel != nil {
        c.clientCancel()
    }
    ctx, cancel := context.WithCancel(context.Background())
    c.clientCtx = ctx
    c.clientCancel = cancel
    c.clientLifecycleMutex.Unlock()

    // Create opc client
    // Up to 3 attempts
    const attempts = 3
    var lastErr error
    for i := 1; i <= attempts; i++ {
        cli, err := opc.NewClient(cfg.EndpointURL)
        if err != nil {
            lastErr = err
            c.Log(fmt.Sprintf("[red]Connect attempt %d/%d: failed to create client: %v[-]", i, attempts, err))
            if i < attempts {
                time.Sleep(1 * time.Second)
                continue
            }
            break
        }
        // Set data change handler and connect
        cli.Handler = c
        // Apply connect timeout if configured
        connectCtx := ctx
        var tCancel context.CancelFunc
        if cfg.ConnectTimeout > 0 {
            d := time.Duration(cfg.ConnectTimeout*1000) * time.Millisecond
            connectCtx, tCancel = context.WithTimeout(ctx, d)
        }
        err = cli.Connect(connectCtx)
        if tCancel != nil {
            tCancel()
        }
        if err != nil {
            lastErr = err
            // Detect timeout: context deadline or errors implementing Timeout() bool
            isTimeout := errors.Is(err, context.DeadlineExceeded)
            if !isTimeout {
                if te, ok := any(err).(interface{ Timeout() bool }); ok && te.Timeout() {
                    isTimeout = true
                }
            }
            if isTimeout {
                c.Log(fmt.Sprintf("[red]Connect attempt %d/%d timeout after %.1fs to %s[-]", i, attempts, cfg.ConnectTimeout, cfg.EndpointURL))
            } else {
                c.Log(fmt.Sprintf("[red]Connect attempt %d/%d failed: %v[-]", i, attempts, err))
            }
            // Best effort disconnect if Connect partially established
            _ = cli.Disconnect(context.Background())
            if i < attempts {
                c.Log("[yellow]Retrying connect...[-]")
                time.Sleep(1 * time.Second)
                continue
            }
            break
        }
        // Success
        c.mu.Lock()
        c.client = cli
        c.isConnected = true
        c.isConnecting = false
        c.mu.Unlock()
        c.Log(fmt.Sprintf("[green]Connected to %s[-]", cfg.EndpointURL))
        if c.OnConnectionStateChange != nil {
            c.OnConnectionStateChange(true, cfg.EndpointURL, nil)
        }
        // Start throttled watch list update pump
        c.startWatchUpdatePump()
        // Kick initial browse of RootFolder if available
        go c.Browse("i=84")
        return nil
    }
    // Final failure
    c.mu.Lock()
    c.isConnecting = false
    c.mu.Unlock()
    if lastErr == nil {
        lastErr = fmt.Errorf("connect failed")
    }
    c.Log(fmt.Sprintf("[red]Connect failed after %d attempts: %v[-]", attempts, lastErr))
    if c.OnConnectionStateChange != nil {
        c.OnConnectionStateChange(false, cfg.EndpointURL, lastErr)
    }
    return lastErr
}

func (c *Controller) Disconnect() {
    c.clientLifecycleMutex.Lock()
    if c.clientCancel != nil {
        c.clientCancel()
        c.clientCancel = nil
    }
    if c.client != nil {
        _ = c.client.Disconnect(context.Background())
        c.client = nil
    }
    c.clientCtx = nil
    c.clientLifecycleMutex.Unlock()

    c.mu.Lock()
    c.isConnected = false
    c.isConnecting = false
    c.mu.Unlock()

    // Close and recreate API broadcast channel to notify Hub and future sessions get a fresh channel
    c.mu.Lock()
    oldBroadcast := c.ApiBroadcastChan
    c.ApiBroadcastChan = make(chan *WatchItem, 64)
    c.mu.Unlock()
    if oldBroadcast != nil {
        close(oldBroadcast)
    }

    // Clear all watches (also closes any active subscriptions) and notify UI
    c.RemoveAllWatches()

    // Clear address space caches and browsing flags
    c.addressSpaceMutex.Lock()
    c.addressSpaceNodes = make(map[string]*AddressSpaceNode)
    c.addressSpaceChildren = make(map[string][]string)
    c.addressSpaceMutex.Unlock()
    c.mu.Lock()
    c.browsingNodes = make(map[string]bool)
    c.noChildrenCached = make(map[string]bool)
    c.mu.Unlock()

    c.Log("[yellow]Disconnected[-]")
    if c.OnConnectionStateChange != nil {
        c.OnConnectionStateChange(false, "", nil)
    }
    if c.OnAddressSpaceReset != nil {
        c.OnAddressSpaceReset()
    }
}

// Shutdown stops the API server (if running) and disconnects the OPC UA client, ensuring all state is cleared.
func (c *Controller) Shutdown() {
    // Stop API server
    if c.apiServerCancel != nil {
        c.apiServerCancel()
    }
    c.apiServer = nil
    c.apiServerCtx = nil
    c.apiServerCancel = nil

    // Disconnect OPC UA client and clear state
    c.Disconnect()
}

func (c *Controller) GetApiBroadcastChan() chan *WatchItem { return c.ApiBroadcastChan }

func (c *Controller) GetClientContext() context.Context { return c.clientCtx }

// ... (rest of the code remains the same)
func (c *Controller) Browse(parentID string) {
    // Prevent duplicate browse for the same node
    c.mu.Lock()
    if c.browsingNodes[parentID] {
        c.mu.Unlock()
        return
    }
    c.browsingNodes[parentID] = true
    ctx := c.clientCtx
    client := c.client
    c.mu.Unlock()

    // Validate state
    if client == nil || ctx == nil {
        c.Log(fmt.Sprintf("[red]Browse aborted for %s: client not connected[-]", parentID))
        c.mu.Lock()
        c.browsingNodes[parentID] = false
        c.mu.Unlock()
        return
    }

    // Parse the parent node id
    nID, err := ua.ParseNodeID(parentID)
    if err != nil {
        c.Log(fmt.Sprintf("[red]Invalid NodeID '%s': %v[-]", parentID, err))
        c.mu.Lock()
        c.browsingNodes[parentID] = false
        c.mu.Unlock()
        return
    }

    // Perform browse with timeout
    browseCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    refs, err := client.Browse(browseCtx, nID)
    if err != nil {
        c.Log(fmt.Sprintf("[red]Browse failed for %s: %v[-]", parentID, err))
        c.mu.Lock()
        c.browsingNodes[parentID] = false
        c.mu.Unlock()
        return
    }

    // Build children list and node entries
    children := make([]string, 0, len(refs))
    nodes := make(map[string]*AddressSpaceNode, len(refs))
    for _, ref := range refs {
        if ref == nil || ref.NodeID == nil {
            continue
        }
        var childID string
        // Prefer concrete NodeID if available
        if ref.NodeID.NodeID != nil {
            childID = ref.NodeID.NodeID.String()
        } else {
            // Fallback to expanded form string
            childID = ref.NodeID.String()
        }
        if childID == "" {
            continue
        }
        name := ""
        if ref.DisplayName.Text != "" {
            name = ref.DisplayName.Text
        } else {
            name = childID
        }

        hasChildren := ref.NodeClass != ua.NodeClassVariable && ref.NodeClass != ua.NodeClassMethod
        nodes[childID] = &AddressSpaceNode{
            NodeID:      childID,
            Name:        name,
            NodeClass:   ref.NodeClass,
            HasChildren: hasChildren,
        }
        children = append(children, childID)
    }

    // Sort children by name for stable UI ordering
    sort.Slice(children, func(i, j int) bool {
        return nodes[children[i]].Name < nodes[children[j]].Name
    })

    // Commit to controller caches
    c.addressSpaceMutex.Lock()
    for id, n := range nodes {
        c.addressSpaceNodes[id] = n
    }
    c.addressSpaceChildren[parentID] = children
    c.addressSpaceMutex.Unlock()

    // Notify UI there are updates for this parent
    select {
    case c.AddressSpaceUpdateChan <- parentID:
    default:
    }

    // Clear browsing flag
    c.mu.Lock()
    c.browsingNodes[parentID] = false
    c.mu.Unlock()
}

func (c *Controller) HasBrowseBeenPerformed(nodeID string) bool {
    c.addressSpaceMutex.RLock()
    _, ok := c.addressSpaceChildren[nodeID]
    c.addressSpaceMutex.RUnlock()
    return ok
}

func (c *Controller) IsBrowsing(nodeID string) bool {
    c.mu.RLock()
    b := c.browsingNodes[nodeID]
    c.mu.RUnlock()
    return b
}

func (c *Controller) GetAddressSpaceChildren(parentID string) []string {
    c.addressSpaceMutex.RLock()
    ch := append([]string(nil), c.addressSpaceChildren[parentID]...)
    c.addressSpaceMutex.RUnlock()
    return ch
}

func (c *Controller) GetNode(id string) *AddressSpaceNode {
    c.addressSpaceMutex.RLock()
    n := c.addressSpaceNodes[id]
    c.addressSpaceMutex.RUnlock()
    return n
}

// CollectVariableNodes collects Variable-class nodes under the given parent. If parentID is empty,
// it attempts to walk the entire known address space starting from RootFolder (i=84).
// It performs best-effort browsing on demand. It respects connection state and client context.
func (c *Controller) CollectVariableNodes(parentID string, recursive bool) ([]*ExportTag, error) {
    // Connection gating
    c.mu.RLock()
    ctx := c.clientCtx
    cli := c.client
    c.mu.RUnlock()
    if cli == nil || ctx == nil {
        return nil, fmt.Errorf("not connected")
    }

    // Determine start IDs
    startIDs := []string{}
    if parentID == "" {
        startIDs = []string{"i=84"} // RootFolder
    } else {
        startIDs = []string{parentID}
    }

    // Iterative BFS to avoid deep recursion and leaks
    queue := make([]string, 0, 64)
    visited := make(map[string]bool)
    queue = append(queue, startIDs...)

    tags := make([]*ExportTag, 0, 256)
    deadline := time.After(30 * time.Second) // safeguard

    for len(queue) > 0 {
        select {
        case <-ctx.Done():
            return tags, ctx.Err()
        case <-deadline:
            // Time-guard to prevent excessive blocking
            return tags, fmt.Errorf("export traversal timeout")
        default:
        }

        id := queue[0]
        queue = queue[1:]
        if visited[id] {
            continue
        }
        visited[id] = true

        // Ensure we have children cached; if unknown and recursive, try to browse
        ch := c.GetAddressSpaceChildren(id)
        if recursive && len(ch) == 0 {
            // best effort browse to populate
            c.Browse(id)
            // small wait to allow browse to complete
            time.Sleep(20 * time.Millisecond)
            ch = c.GetAddressSpaceChildren(id)
        }

        // If this node is Variable, add to tags
        if n := c.GetNode(id); n != nil {
            if n.NodeClass == ua.NodeClassVariable {
                // Best-effort attributes
                var dt, desc string
                if attrs, err := c.ReadNodeAttributes(id); err == nil && attrs != nil {
                    dt = attrs.DataType
                    desc = attrs.Description
                }
                tags = append(tags, &ExportTag{
                    NodeID:      id,
                    Name:        n.Name,
                    DataType:    dt,
                    Description: desc,
                })
            }
        }

        if recursive {
            // Enqueue children
            for _, child := range ch {
                if !visited[child] {
                    queue = append(queue, child)
                }
            }
        }
    }

    return tags, nil
}

func (c *Controller) AddWatch(nodeID string) {
    // Validate connection first
    c.mu.RLock()
    cli := c.client
    c.mu.RUnlock()
    if cli == nil {
        c.Log(fmt.Sprintf("[red]AddWatch failed: not connected (node %s)[-]", nodeID))
        return
    }

    // Create entry or return if exists
    c.mu.Lock()
    if _, exists := c.watchItems[nodeID]; exists {
        c.mu.Unlock()
        return
    }
    wi := &WatchItem{NodeID: nodeID}
    c.watchItems[nodeID] = wi
    c.mu.Unlock()

    // Populate fields from attributes (best-effort)
    if attrs, err := c.ReadNodeAttributes(nodeID); err == nil && attrs != nil {
        c.mu.Lock()
        if it, ok := c.watchItems[nodeID]; ok {
            it.Name = attrs.Name
            it.DataType = attrs.DataType
            it.Value = attrs.Value
            it.Timestamp = time.Now().Format("15:04:05.000")
        }
        c.mu.Unlock()
    }

    // Start monitoring value changes
    sub, err := cli.MonitorItem(nodeID)
    if err != nil {
        c.Log(fmt.Sprintf("[red]Failed to monitor %s: %v[-]", nodeID, err))
    } else {
        c.mu.Lock()
        if it, ok := c.watchItems[nodeID]; ok {
            it.subHandle = sub
        }
        c.mu.Unlock()
        c.Log(fmt.Sprintf("[green]Monitoring %s started[-]", nodeID))
    }

    // Push snapshot to UI
    c.mu.RLock()
    items := make([]*WatchItem, 0, len(c.watchItems))
    for _, it := range c.watchItems {
        items = append(items, it)
    }
    // Stable order by NodeID
    sort.Slice(items, func(i, j int) bool { return items[i].NodeID < items[j].NodeID })
    cb := c.OnWatchListUpdate
    // Prepare API broadcast of the newly added item (shallow copy)
    if it, ok := c.watchItems[nodeID]; ok {
        msg := *it
        // do not include subHandle in broadcast
        msg.subHandle = nil
        broadcast := c.ApiBroadcastChan
        go func(m *WatchItem, ch chan *WatchItem) {
            select { case ch <- m: default: }
        }(&msg, broadcast)
    }
    c.mu.RUnlock()
    if cb != nil {
        cb(items)
    }
}

func (c *Controller) GetClientForExport() *opc.Client {
    c.mu.RLock()
    cli := c.client
    c.mu.RUnlock()
    return cli
}

// SetApiStarter injects the API server start function to avoid import cycles.
func (c *Controller) SetApiStarter(starter ApiServerStarter) {
    c.apiStarter = starter
}

// SetApiStatus allows the UI to bind to a status string owned by the controller.
func (c *Controller) SetApiStatus(ptr *string) { c.apiStatus = ptr }

// UpdateApiServerState starts/stops the API server based on cfg.ApiEnabled and port changes.
func (c *Controller) UpdateApiServerState(cfg *opc.Config) {
    // Save the latest config
    c.currentConfig = cfg

    if c.apiStarter == nil || c.apiStatus == nil {
        return
    }
    // If disabled, ensure server is stopped
    if cfg == nil || !cfg.ApiEnabled {
        if c.apiServerCancel != nil {
            c.apiServerCancel()
        }
        c.apiServer = nil
        c.apiServerCtx = nil
        c.apiServerCancel = nil
        if c.apiStatus != nil {
            *c.apiStatus = "API Disabled"
        }
        return
    }

    // If enabled and server not running, start it
    needStart := c.apiServer == nil
    // Or restart if port changed
    if !needStart && c.currentConfig != nil {
        // Compare server address via cfg.ApiPort; we don't have a direct getter from http.Server
        // so conservatively restart on any Update when enabled.
        needStart = true
    }
    if needStart {
        if c.apiServerCancel != nil {
            c.apiServerCancel()
        }
        ctx, cancel := context.WithCancel(context.Background())
        c.apiServerCtx = ctx
        c.apiServerCancel = cancel
        c.apiServer = c.apiStarter(ctx, c, c.apiStatus, cfg)
    }
}

func (c *Controller) HandleDataChange(nodeID string, dv *ua.DataValue) {
    c.mu.Lock()
    item, ok := c.watchItems[nodeID]
    if !ok {
        c.mu.Unlock()
        return
    }
    if dv == nil {
        item.Value = "<error: no data>"
        item.Timestamp = time.Now().Format("15:04:05.000")
        item.Severity = "Bad"
        // fall-through to notify UI below
    }
    if dv.Value != nil {
        item.Value = formatValue(dv.Value, item.DataType)
    } else {
        item.Value = "<nil>"
    }
    item.Timestamp = time.Now().Format("15:04:05.000")
    sev, symName, subCode, structChanged, semChanged, infoBits, rawCode := decodeStatusCode(dv.Status)
    item.Severity = sev
    item.SymbolicName = symName
    item.SubCode = subCode
    item.StructureChanged = structChanged
    item.SemanticsChanged = semChanged
    item.InfoBits = infoBits
    item.RawCode = rawCode
    // Snapshot for UI
    items := make([]*WatchItem, 0, len(c.watchItems))
    for _, wi := range c.watchItems { items = append(items, wi) }
    sort.Slice(items, func(i, j int) bool { return items[i].NodeID < items[j].NodeID })
    update := c.OnWatchListUpdate
    // Prepare API broadcast message (shallow copy)
    msg := *item
    msg.subHandle = nil
    broadcast := c.ApiBroadcastChan
    c.mu.Unlock()

    // UI update
    if update != nil {
        update(items)
    }
    // Non-blocking API broadcast
    select {
    case broadcast <- &msg:
    default:
        // drop if channel is full to avoid blocking
    }
}

func (c *Controller) RemoveWatch(nodeID string) {
	var subToClose *opc.Subscription

	c.mu.Lock()
	item, ok := c.watchItems[nodeID]
	if !ok {
		c.mu.Unlock()
		return
	}
	subToClose = item.subHandle
	delete(c.watchItems, nodeID)
	// Prepare snapshot for UI update after unlock
	itemsToUpdate := make([]*WatchItem, 0, len(c.watchItems))
	for _, wi := range c.watchItems {
		itemsToUpdate = append(itemsToUpdate, wi)
	}
	sort.Slice(itemsToUpdate, func(i, j int) bool { return itemsToUpdate[i].NodeID < itemsToUpdate[j].NodeID })
	updateFunc := c.OnWatchListUpdate
	c.mu.Unlock()

	if subToClose != nil {
		if err := subToClose.Close(); err != nil {
			c.Log(fmt.Sprintf("[red]Failed to unmonitor %s: %v[-]", nodeID, err))
		} else {
			c.Log(fmt.Sprintf("[green]Stopped monitoring %s[-]", nodeID))
		}
	}

	// Notify UI of updated watch list
	if updateFunc != nil {
		updateFunc(itemsToUpdate)
	}
}

func (c *Controller) RemoveAllWatches() {
	c.mu.Lock()
	// collect subs to close and clear map
	subs := make([]*opc.Subscription, 0, len(c.watchItems))
	for _, item := range c.watchItems {
		subs = append(subs, item.subHandle)
	}
	c.watchItems = make(map[string]*WatchItem)
	updateFunc := c.OnWatchListUpdate
	c.mu.Unlock()

	// close subs outside lock
	for _, sub := range subs {
		if sub != nil {
			_ = sub.Close()
		}
	}
	c.Log("[green]Cleared all items from watch list[-]")

	// notify UI
	if updateFunc != nil {
		updateFunc([]*WatchItem{})
	}

}

func convertStringToType(valueStr, dataType string) (interface{}, error) {
    switch strings.ToLower(strings.TrimSpace(dataType)) {
    case "boolean", "bool":
        return strconv.ParseBool(valueStr)
    case "sbyte":
        v, err := strconv.ParseInt(valueStr, 10, 8)
        if err != nil {
            return nil, err
        }
        return int8(v), nil
    case "byte":
        v, err := strconv.ParseUint(valueStr, 10, 8)
        if err != nil {
            return nil, err
        }
        return uint8(v), nil
    case "bytestring":
        // Normalize common inputs for ByteString:
        // - Allow hex with or without spaces/commas, with optional 0x prefix
        // - Support explicit prefixes: ascii:, text:, hex:
        // - Example accepted forms:
        //   "4345de", "43 45 DE", "0x43 0x45 0xDE", "43,45,DE", "ascii:abcd", "hex:43 45 DE"
        s := strings.TrimSpace(valueStr)
        if s == "" {
            // Empty string -> empty non-nil ByteString (not null)
            return []byte{}, nil
        }
        // Explicit ASCII/text prefix forces raw bytes
        if strings.HasPrefix(strings.ToLower(s), "ascii:") || strings.HasPrefix(strings.ToLower(s), "text:") {
            return []byte(strings.TrimSpace(s[strings.Index(s, ":")+1:])), nil
        }
        // Explicit hex: prefix allowed; strip it then continue normalization
        if strings.HasPrefix(strings.ToLower(s), "hex:") {
            s = strings.TrimSpace(s[4:])
        }
        // Remove separators and 0x/0X prefixes
        // First split on separators, then strip 0x and rejoin
        parts := strings.FieldsFunc(s, func(r rune) bool {
            return r == ' ' || r == ',' || r == ';' || r == ':'
        })
        if len(parts) > 0 {
            for i := range parts {
                parts[i] = strings.TrimPrefix(strings.TrimPrefix(parts[i], "0x"), "0X")
            }
            s = strings.Join(parts, "")
        }
        // If even length and valid hex, decode as bytes
        if len(s)%2 == 0 {
            if b, err := hex.DecodeString(s); err == nil {
                return b, nil
            }
        }
        // Fallback: treat as raw ASCII/UTF-8 bytes
        return []byte(valueStr), nil
    case "int16":
        v, err := strconv.ParseInt(valueStr, 10, 16)
        if err != nil {
            return nil, err
        }
        return int16(v), nil
    case "uint16":
        v, err := strconv.ParseUint(valueStr, 10, 16)
        if err != nil {
            return nil, err
        }
        return uint16(v), nil
    case "int32":
        v, err := strconv.ParseInt(valueStr, 10, 32)
        if err != nil {
            return nil, err
        }
        return int32(v), nil
    case "uint32":
        v, err := strconv.ParseUint(valueStr, 10, 32)
        if err != nil {
            return nil, err
        }
        return uint32(v), nil
    case "int64":
        v, err := strconv.ParseInt(valueStr, 10, 64)
        if err != nil {
            return nil, err
        }
        return v, nil
    case "uint64":
        v, err := strconv.ParseUint(valueStr, 10, 64)
        if err != nil {
            return nil, err
        }
        return v, nil
    case "float", "float32":
        v, err := strconv.ParseFloat(valueStr, 32)
        if err != nil {
            return nil, err
        }
        return float32(v), nil
    case "double", "float64":
        v, err := strconv.ParseFloat(valueStr, 64)
        if err != nil {
            return nil, err
        }
        return v, nil
    case "datetime":
        s := strings.TrimSpace(valueStr)
        if strings.EqualFold(s, "now") {
            return time.Now(), nil
        }
        // Try common layouts
        layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04:05.000"}
        var t time.Time
        var perr error
        for _, layout := range layouts {
            if tt, err := time.Parse(layout, s); err == nil {
                t = tt
                perr = nil
                break
            } else {
                perr = err
            }
        }
        if perr != nil {
            return nil, perr
        }
        return t, nil
    case "localizedtext":
        // Accept "text" or "locale|text"
        parts := strings.SplitN(valueStr, "|", 2)
        if len(parts) == 2 {
            return ua.LocalizedText{Locale: strings.TrimSpace(parts[0]), Text: strings.TrimSpace(parts[1])}, nil
        }
        return ua.LocalizedText{Text: valueStr}, nil
    case "string":
        return valueStr, nil
    default:
        return nil, fmt.Errorf("unsupported data type: %s", dataType)
    }
}

func (c *Controller) WriteValue(nodeID, dataType, valueStr string) {
    c.mu.RLock()
	if c.client == nil {
		c.Log("[red]Not connected. Cannot write value[-]")
		c.mu.RUnlock()
		return
	}
	client := c.client
	c.mu.RUnlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				c.Log(fmt.Sprintf("[red]WriteValue panic recovered for %s: %v[-]", nodeID, r))
			}
		}()

		// Basic validation of NodeID format for clearer error logging
		if _, err := ua.ParseNodeID(nodeID); err != nil {
			c.Log(fmt.Sprintf("[red]Invalid NodeID '%s': %v[-]", nodeID, err))
			return
		}

		        // Read the authoritative DataType/ValueRank from server to avoid type mismatch
        serverDT := ""
        serverVR := -1
        if a, err := c.ReadNodeAttributes(nodeID); err == nil && a != nil {
            // Gate on write access
            if !strings.Contains(strings.ToLower(a.AccessLevel), "write") {
                c.Log(fmt.Sprintf("[red]Node %s is not writable (AccessLevel=%s). Abort write.[-]", nodeID, a.AccessLevel))
                return
            }
            if a.DataType != "" {
                serverDT = a.DataType
            }
            serverVR = a.ValueRank
        }
        if serverDT != "" {
            if !strings.EqualFold(dataType, serverDT) {
                c.Log(fmt.Sprintf("[yellow]Overriding provided DataType '%s' with server-reported '%s'[-]", dataType, serverDT))
            }
            dataType = serverDT
        }
        if serverVR >= 0 {
            c.Log(fmt.Sprintf("[yellow]Server reports ValueRank=%d (array). Input will be parsed as an array.[-]", serverVR))
        }
        c.Log(fmt.Sprintf("[cyan]Resolved DataType=%s, ValueRank=%d[-]", dataType, serverVR))

        // Probe actual variant type by reading current value (helps when attribute DataType is misleading)
        var preferScalarGoType reflect.Kind
        if serverVR < 0 { // only meaningful for scalar
            func() {
                ctx0, cancel0 := context.WithTimeout(context.Background(), 2*time.Second)
                defer cancel0()
                // read only Value attribute
                vals, rerr := client.ReadAttributes(ctx0, nodeID, ua.AttributeIDValue)
                if rerr == nil && len(vals) == 1 && vals[0] != nil && vals[0].Value != nil {
                    cur := vals[0].Value.Value()
                    if cur != nil {
                        preferScalarGoType = reflect.TypeOf(cur).Kind()
                        c.Log(fmt.Sprintf("[cyan]Actual current Value GoType=%T, Kind=%s, Val=%v[-]", cur, preferScalarGoType, cur))
                    }
                }
            }()
        }

        // If array is expected, parse CSV or bracketed input into a typed slice
        var writeValue interface{}
        var err error
        if serverVR >= 0 { // array or matrix
            // normalize input like "[1,2,3]" or "1,2,3"
            s := strings.TrimSpace(valueStr)
            s = strings.TrimPrefix(s, "[")
            s = strings.TrimSuffix(s, "]")
            parts := strings.Split(s, ",")
            // helper to trim each
            trim := func(ss []string) []string {
                out := make([]string, 0, len(ss))
                for _, p := range ss {
                    t := strings.TrimSpace(p)
                    if t != "" {
                        out = append(out, t)
                    }
                }
                return out
            }
            items := trim(parts)
            if len(items) == 0 {
                c.Log("[red]Empty array input for array-typed node[-]")
                return
            }
            dt := strings.ToLower(strings.TrimSpace(dataType))
            switch dt {
            case "float", "float32":
                arr := make([]float32, 0, len(items))
                for _, it := range items {
                    v, perr := strconv.ParseFloat(it, 32)
                    if perr != nil {
                        c.Log(fmt.Sprintf("[red]Failed to parse '%s' as float32: %v[-]", it, perr))
                        return
                    }
                    arr = append(arr, float32(v))
                }
                writeValue = arr
            case "double", "float64":
                arr := make([]float64, 0, len(items))
                for _, it := range items {
                    v, perr := strconv.ParseFloat(it, 64)
                    if perr != nil {
                        c.Log(fmt.Sprintf("[red]Failed to parse '%s' as float64: %v[-]", it, perr))
                        return
                    }
                    arr = append(arr, v)
                }
                writeValue = arr
            case "int16":
                arr := make([]int16, 0, len(items))
                for _, it := range items {
                    v, perr := strconv.ParseInt(it, 10, 16)
                    if perr != nil {
                        c.Log(fmt.Sprintf("[red]Failed to parse '%s' as int16: %v[-]", it, perr))
                        return
                    }
                    arr = append(arr, int16(v))
                }
                writeValue = arr
            case "uint16":
                arr := make([]uint16, 0, len(items))
                for _, it := range items {
                    v, perr := strconv.ParseUint(it, 10, 16)
                    if perr != nil {
                        c.Log(fmt.Sprintf("[red]Failed to parse '%s' as uint16: %v[-]", it, perr))
                        return
                    }
                    arr = append(arr, uint16(v))
                }
                writeValue = arr
            case "int32":
                arr := make([]int32, 0, len(items))
                for _, it := range items {
                    v, perr := strconv.ParseInt(it, 10, 32)
                    if perr != nil {
                        c.Log(fmt.Sprintf("[red]Failed to parse '%s' as int32: %v[-]", it, perr))
                        return
                    }
                    arr = append(arr, int32(v))
                }
                writeValue = arr
            case "uint32":
                arr := make([]uint32, 0, len(items))
                for _, it := range items {
                    v, perr := strconv.ParseUint(it, 10, 32)
                    if perr != nil {
                        c.Log(fmt.Sprintf("[red]Failed to parse '%s' as uint32: %v[-]", it, perr))
                        return
                    }
                    arr = append(arr, uint32(v))
                }
                writeValue = arr
            case "int64":
                arr := make([]int64, 0, len(items))
                for _, it := range items {
                    v, perr := strconv.ParseInt(it, 10, 64)
                    if perr != nil {
                        c.Log(fmt.Sprintf("[red]Failed to parse '%s' as int64: %v[-]", it, perr))
                        return
                    }
                    arr = append(arr, v)
                }
                writeValue = arr
            case "uint64":
                arr := make([]uint64, 0, len(items))
                for _, it := range items {
                    v, perr := strconv.ParseUint(it, 10, 64)
                    if perr != nil {
                        c.Log(fmt.Sprintf("[red]Failed to parse '%s' as uint64: %v[-]", it, perr))
                        return
                    }
                    arr = append(arr, v)
                }
                writeValue = arr
            case "boolean", "bool":
                arr := make([]bool, 0, len(items))
                for _, it := range items {
                    v, perr := strconv.ParseBool(it)
                    if perr != nil {
                        c.Log(fmt.Sprintf("[red]Failed to parse '%s' as bool: %v[-]", it, perr))
                        return
                    }
                    arr = append(arr, v)
                }
                writeValue = arr
            case "string":
                // items already strings
                writeValue = items
            case "localizedtext":
                arr := make([]ua.LocalizedText, 0, len(items))
                for _, it := range items {
                    parts := strings.SplitN(it, "|", 2)
                    if len(parts) == 2 {
                        arr = append(arr, ua.LocalizedText{Locale: strings.TrimSpace(parts[0]), Text: strings.TrimSpace(parts[1])})
                    } else {
                        arr = append(arr, ua.LocalizedText{Text: it})
                    }
                }
                writeValue = arr
            case "datetime":
                arr := make([]time.Time, 0, len(items))
                for _, it := range items {
                    // Accept RFC3339 or common format
                    var t time.Time
                    var perr error
                    layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04:05.000"}
                    for _, layout := range layouts {
                        if tt, err := time.Parse(layout, it); err == nil {
                            t = tt
                            perr = nil
                            break
                        } else {
                            perr = err
                        }
                    }
                    if perr != nil {
                        c.Log(fmt.Sprintf("[red]Failed to parse '%s' as DateTime: %v[-]", it, perr))
                        return
                    }
                    arr = append(arr, t)
                }
                writeValue = arr
            default:
                c.Log(fmt.Sprintf("[red]Array writes for type '%s' are not implemented yet[-]", dataType))
                return
            }
        } else {
            // If we detected actual scalar Go kind and it is numeric, coerce to that exact width first
            switch preferScalarGoType {
            case reflect.Float32:
                writeValue, err = convertStringToType(valueStr, "float32")
            case reflect.Float64:
                writeValue, err = convertStringToType(valueStr, "float64")
            case reflect.Int16:
                writeValue, err = convertStringToType(valueStr, "int16")
            case reflect.Int32:
                writeValue, err = convertStringToType(valueStr, "int32")
            case reflect.Int64:
                writeValue, err = convertStringToType(valueStr, "int64")
            case reflect.Uint16:
                writeValue, err = convertStringToType(valueStr, "uint16")
            case reflect.Uint32:
                writeValue, err = convertStringToType(valueStr, "uint32")
            case reflect.Uint64:
                writeValue, err = convertStringToType(valueStr, "uint64")
            case reflect.Bool:
                writeValue, err = convertStringToType(valueStr, "bool")
            default:
                writeValue, err = convertStringToType(valueStr, dataType)
            }
        }
        if err != nil {
            c.Log(fmt.Sprintf("[red]Failed to parse value '%s' for type %s: %v[-]", valueStr, dataType, err))
            return
        }
 
        c.Log(fmt.Sprintf("Attempting to write to NodeID %s. Value: %v (GoType: %T, Kind: %s)", nodeID, writeValue, writeValue, reflect.TypeOf(writeValue).Kind()))

        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        // helper: perform write and verify by reading back Value
        tryWrite := func(val interface{}) (bool, error) {
            if werr := client.WriteValue(ctx, nodeID, val); werr != nil {
                return false, werr
            }
            // verify
            vctx, vcancel := context.WithTimeout(context.Background(), 2*time.Second)
            defer vcancel()
            vals, rerr := client.ReadAttributes(vctx, nodeID, ua.AttributeIDValue, ua.AttributeIDDataType)
            if rerr == nil && len(vals) >= 1 && vals[0] != nil {
                // DataType might be at index 1 if returned
                var dtName string
                if len(vals) >= 2 && vals[1] != nil {
                    if nid, ok := vals[1].Value.Value().(*ua.NodeID); ok {
                        dtName = builtinTypeName(nid)
                    }
                }
                c.Log(fmt.Sprintf("[green]Write success. Server Value=%v DataType=%s[-]", vals[0].Value.Value(), dtName))
            }
            return true, nil
        }

        // Perform write
        if ok, err := tryWrite(writeValue); !ok {
            c.Log(fmt.Sprintf("[red]Failed to write to %s: %v[-]", nodeID, err))
            lower := strings.ToLower(err.Error())
            // Retry on type mismatch
            if strings.Contains(lower, "typemismatch") || strings.Contains(lower, "bad_type") {
                // A) If we attempted scalar, try as single-element array
                if reflect.ValueOf(writeValue).Kind() != reflect.Slice {
                    // A0) If server provided DataType differs from what we sent, try reconverting to server DataType
                    if dataType != "" {
                        c.Log(fmt.Sprintf("[yellow]TypeMismatch: retry using server DataType '%s' as scalar...[-]", dataType))
                        if coerced, ferr := convertStringToType(valueStr, dataType); ferr == nil {
                            if ok, _ := tryWrite(coerced); ok {
                                c.Log(fmt.Sprintf("[yellow]Retried using server DataType '%s' and succeeded for %s[-]", dataType, nodeID))
                                return
                            } else {
                                c.Log(fmt.Sprintf("[red]Retry using server DataType '%s' failed[-]", dataType))
                            }
                        } else {
                            c.Log(fmt.Sprintf("[red]Cannot coerce input to server DataType '%s': %v[-]", dataType, ferr))
                        }
                    }
                    s := strings.TrimSpace(valueStr)
                    s = strings.TrimPrefix(s, "[")
                    s = strings.TrimSuffix(s, "]")
                    if s != "" {
                        items := []string{s}
                        dt := strings.ToLower(strings.TrimSpace(dataType))
                        var arr interface{}
                        var buildErr error
                        switch dt {
                        case "float", "float32":
                            v, perr := strconv.ParseFloat(items[0], 32); buildErr = perr; if buildErr == nil { arr = []float32{float32(v)} }
                        case "double", "float64":
                            v, perr := strconv.ParseFloat(items[0], 64); buildErr = perr; if buildErr == nil { arr = []float64{v} }
                        case "int16":
                            v, perr := strconv.ParseInt(items[0], 10, 16); buildErr = perr; if buildErr == nil { arr = []int16{int16(v)} }
                        case "uint16":
                            v, perr := strconv.ParseUint(items[0], 10, 16); buildErr = perr; if buildErr == nil { arr = []uint16{uint16(v)} }
                        case "int32":
                            v, perr := strconv.ParseInt(items[0], 10, 32); buildErr = perr; if buildErr == nil { arr = []int32{int32(v)} }
                        case "uint32":
                            v, perr := strconv.ParseUint(items[0], 10, 32); buildErr = perr; if buildErr == nil { arr = []uint32{uint32(v)} }
                        case "int64":
                            v, perr := strconv.ParseInt(items[0], 10, 64); buildErr = perr; if buildErr == nil { arr = []int64{v} }
                        case "uint64":
                            v, perr := strconv.ParseUint(items[0], 10, 64); buildErr = perr; if buildErr == nil { arr = []uint64{v} }
                        case "boolean", "bool":
                            v, perr := strconv.ParseBool(items[0]); buildErr = perr; if buildErr == nil { arr = []bool{v} }
                        case "string":
                            arr = []string{items[0]}
                        case "localizedtext":
                            parts := strings.SplitN(items[0], "|", 2)
                            if len(parts) == 2 {
                                arr = []ua.LocalizedText{{Locale: strings.TrimSpace(parts[0]), Text: strings.TrimSpace(parts[1])}}
                            } else {
                                arr = []ua.LocalizedText{{Text: items[0]}}
                            }
                        case "datetime":
                            t, perr := time.Parse("2006-01-02 15:04:05.999999999", items[0]); buildErr = perr; if buildErr == nil { arr = []time.Time{t} }
                        }
                        if buildErr == nil && arr != nil {
                            c.Log("[yellow]TypeMismatch: retry as single-element array...[-]")
                            if ok, _ := tryWrite(arr); ok {
                                c.Log(fmt.Sprintf("[yellow]Retried as array and succeeded for %s[-]", nodeID))
                                return
                            } else {
                                c.Log("[red]Array retry failed[-]")
                            }
                        }
                    }
                }
                // B) scalar float64 -> float32 retry
                if _, ok := writeValue.(float64); ok {
                    c.Log("[yellow]TypeMismatch: retry scalar float64 as float32...[-]")
                    if fv, ferr := convertStringToType(valueStr, "float32"); ferr == nil {
                        if ok, _ := tryWrite(fv); ok {
                            c.Log(fmt.Sprintf("[yellow]Retried as Float32 and succeeded for %s[-]", nodeID))
                            return
                        } else {
                            c.Log("[red]Float32 retry failed[-]")
                        }
                    } else {
                        c.Log(fmt.Sprintf("[red]Cannot convert to float32 for retry: %v[-]", ferr))
                    }
                }
                // Final exhaustive fallback matrix if still failing
                candidates := []string{"bytestring","float64","float32","int64","int32","int16","uint64","uint32","uint16","bool","string"}
                for _, tname := range candidates {
                    // scalar attempt
                    if v, perr := convertStringToType(valueStr, tname); perr == nil {
                        c.Log(fmt.Sprintf("[yellow]Fallback: try scalar as %s...[-]", tname))
                        if ok, _ := tryWrite(v); ok {
                            c.Log(fmt.Sprintf("[green]Fallback success as scalar %s for %s[-]", tname, nodeID))
                            return
                        } else {
                            c.Log(fmt.Sprintf("[red]Fallback scalar %s failed[-]", tname))
                        }
                    }
                    // array attempt [single element]
                    if v, perr := convertStringToType(valueStr, tname); perr == nil {
                        arr := []interface{}{v}
                        c.Log(fmt.Sprintf("[yellow]Fallback: try single-element array as %s...[-]", tname))
                        if ok, _ := tryWrite(arr); ok {
                            c.Log(fmt.Sprintf("[green]Fallback success as array %s for %s[-]", tname, nodeID))
                            return
                        } else {
                            c.Log(fmt.Sprintf("[red]Fallback array %s failed[-]", tname))
                        }
                    }
                }
                c.Log("[red]All fallback attempts exhausted. Write failed.[-]")
            }
            return
        }
        c.Log(fmt.Sprintf("[green]Write to %s succeeded[-]", nodeID))
    }()
}

func (c *Controller) ReadNodeAttributes(nodeID string) (*NodeAttributes, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, errors.New("not connected")
	}

	if _, err := ua.ParseNodeID(nodeID); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	attrsToRead := []ua.AttributeID{
		ua.AttributeIDNodeID,
		ua.AttributeIDNodeClass,
		ua.AttributeIDDisplayName,
		ua.AttributeIDDescription,
		ua.AttributeIDAccessLevel,
		ua.AttributeIDUserAccessLevel,
		ua.AttributeIDDataType,
		ua.AttributeIDValue,
		ua.AttributeIDValueRank,
		ua.AttributeIDArrayDimensions,
	}

    results, err := client.ReadAttributes(ctx, nodeID, attrsToRead...)
    if err != nil {
        return nil, err
    }

    attrs := &NodeAttributes{ValueRank: -1}
    var rawValue *ua.Variant
    var levelValue uint32
    var userLevelValue uint32

    for i, res := range results {
        if res == nil || res.Status != ua.StatusOK {
            continue
        }
        attrID := attrsToRead[i]
        switch attrID {
        case ua.AttributeIDNodeID:
            if id, ok := res.Value.Value().(*ua.NodeID); ok {
                attrs.NodeID = id.String()
            }
        case ua.AttributeIDNodeClass:
            switch v := res.Value.Value().(type) {
            case ua.NodeClass:
                attrs.NodeClass = "NodeClass" + v.String()
            case uint32:
                attrs.NodeClass = "NodeClass" + ua.NodeClass(v).String()
            case uint16:
                attrs.NodeClass = "NodeClass" + ua.NodeClass(v).String()
            case int32:
                attrs.NodeClass = "NodeClass" + ua.NodeClass(uint32(v)).String()
            case int:
                attrs.NodeClass = "NodeClass" + ua.NodeClass(uint32(v)).String()
            }
        case ua.AttributeIDDisplayName:
            if lt, ok := res.Value.Value().(ua.LocalizedText); ok {
                attrs.Name = lt.Text
            } else if lt, ok := res.Value.Value().(*ua.LocalizedText); ok && lt != nil {
                attrs.Name = lt.Text
            }
        case ua.AttributeIDDescription:
            if lt, ok := res.Value.Value().(ua.LocalizedText); ok {
                attrs.Description = lt.Text
            } else if lt, ok := res.Value.Value().(*ua.LocalizedText); ok && lt != nil {
                attrs.Description = lt.Text
            }
        case ua.AttributeIDAccessLevel:
            switch v := res.Value.Value().(type) {
            case uint8:
                levelValue = uint32(v)
            case uint16:
                levelValue = uint32(v)
            case uint32:
                levelValue = v
            case int32:
                levelValue = uint32(v)
            }
        case ua.AttributeIDUserAccessLevel:
            switch v := res.Value.Value().(type) {
            case uint8:
                userLevelValue = uint32(v)
            case uint16:
                userLevelValue = uint32(v)
            case uint32:
                userLevelValue = v
            case int32:
                userLevelValue = uint32(v)
            }
        case ua.AttributeIDDataType:
            if dt, ok := res.Value.Value().(*ua.NodeID); ok {
                attrs.DataType = builtinTypeName(dt)
            }
        case ua.AttributeIDValue:
            rawValue = res.Value
        case ua.AttributeIDValueRank:
            switch v := res.Value.Value().(type) {
            case int32:
                attrs.ValueRank = int(v)
            case int16:
                attrs.ValueRank = int(v)
            case int64:
                attrs.ValueRank = int(v)
            case uint32:
                attrs.ValueRank = int(v)
            case uint16:
                attrs.ValueRank = int(v)
            case uint8:
                attrs.ValueRank = int(v)
            }
        }
    }

    if attrs.NodeID == "" {
        attrs.NodeID = nodeID
    }
    // Prefer UserAccessLevel if provided; fallback to AccessLevel
    if userLevelValue > 0 {
        attrs.AccessLevel = formatAccessLevel(ua.AccessLevelType(userLevelValue))
    } else if levelValue > 0 {
        attrs.AccessLevel = formatAccessLevel(ua.AccessLevelType(levelValue))
    }
    if rawValue != nil {
        attrs.Value = formatValue(rawValue, attrs.DataType)
    }
    if c.OnNodeAttributesUpdate != nil {
        c.OnNodeAttributesUpdate(attrs)
    }
    return attrs, nil
}

// ReadNodeClass reads only the NodeClass for a given node. Some UI code depends on this helper.
func (c *Controller) ReadNodeClass(nodeID string) (ua.NodeClass, error) {
    c.mu.RLock()
    client := c.client
    c.mu.RUnlock()

    if client == nil {
        return 0, errors.New("not connected")
    }
    nID, err := ua.ParseNodeID(nodeID)
    if err != nil {
        return 0, err
    }
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    return client.ReadNodeClass(ctx, nID)
}

func formatValue(variant *ua.Variant, dataType string) string {
    if variant == nil {
        return ""
    }
    switch v := variant.Value().(type) {
    case string:
        return v
    case []byte:
        // If server data type is String, show as text
        if strings.EqualFold(dataType, "string") {
            return string(v)
        }
        // If ByteString, display as contiguous lowercase hex (no spaces) to match UAExpert style
        if strings.EqualFold(dataType, "bytestring") {
            return strings.ToLower(hex.EncodeToString(v))
        }
        // Fallback for other cases: grouped hex with spaces
        return fmt.Sprintf("% X", v)
    case time.Time:
        return v.Format("2006-01-02 15:04:05.000")
    case ua.LocalizedText:
        return v.Text
    case *ua.LocalizedText:
        if v != nil {
            return v.Text
        }
        return ""
    case int, int8, int16, int32, int64,
        uint, uint8, uint16, uint32, uint64,
        float32, float64, bool:
        return fmt.Sprintf("%v", v)
    default:
        return fmt.Sprintf("%v", v)
    }
}

func formatAccessLevel(level ua.AccessLevelType) string {
	var parts []string
	if level&ua.AccessLevelTypeCurrentRead == ua.AccessLevelTypeCurrentRead {
		parts = append(parts, "Read")
	}
	if level&ua.AccessLevelTypeCurrentWrite == ua.AccessLevelTypeCurrentWrite {
		parts = append(parts, "Write")
	}
	if level&ua.AccessLevelTypeHistoryRead == ua.AccessLevelTypeHistoryRead {
		parts = append(parts, "HistoryRead")
	}
	if level&ua.AccessLevelTypeHistoryWrite == ua.AccessLevelTypeHistoryWrite {
		parts = append(parts, "HistoryWrite")
	}
	if level&ua.AccessLevelTypeSemanticChange == ua.AccessLevelTypeSemanticChange {
		parts = append(parts, "SemanticChange")
	}
	if level&ua.AccessLevelTypeStatusWrite == ua.AccessLevelTypeStatusWrite {
		parts = append(parts, "StatusWrite")
	}
	if level&ua.AccessLevelTypeTimestampWrite == ua.AccessLevelTypeTimestampWrite {
		parts = append(parts, "TimestampWrite")
	}
	if len(parts) == 0 {
		return "None"
	}
	return strings.Join(parts, ", ")
}

// 根据NodeID转换为内置数据类型名称
func builtinTypeName(id *ua.NodeID) string {
	if id == nil {
		return ""
	}
	if id.Namespace() != 0 {
		return id.String()
	}
	switch id.IntID() {
	case 1:
		return "Boolean"
	case 2:
		return "SByte"
	case 3:
		return "Byte"
	case 4:
		return "Int16"
	case 5:
		return "UInt16"
	case 6:
		return "Int32"
	case 7:
		return "UInt32"
	case 8:
		return "Int64"
	case 9:
		return "UInt64"
	case 10:
		return "Float"
	case 11:
		return "Double"
	case 12:
		return "String"
	case 13:
		return "DateTime"
	case 14:
		return "Guid"
	case 15:
		return "ByteString"
	case 16:
		return "XmlElement"
	case 17:
		return "NodeId"
	case 18:
		return "ExpandedNodeId"
	case 19:
		return "StatusCode"
	case 20:
		return "QualifiedName"
	case 21:
		return "LocalizedText"
	case 22:
		return "ExtensionObject"
	case 23:
		return "DataValue"
	case 24:
		return "Variant"
	case 25:
		return "DiagnosticInfo"
	default:
		return id.String()
	}
}

// 解析状态码详细信息
func decodeStatusCode(
	status ua.StatusCode,
) (
	sev string,
	symName string,
	subCode uint16,
	structChanged bool,
	semChanged bool,
	infoBits uint16,
	rawCode string,
) {
	rawVal := uint32(status)
	rawCode = fmt.Sprintf("0x%08X", rawVal)

	switch (rawVal >> 30) & 0x3 {
	case 0:
		sev = "Good"
	case 1:
		sev = "Uncertain"
	case 2:
		sev = "Bad"
	default:
		sev = "Unknown"
	}

	subCode = uint16((rawVal >> 16) & 0x3FFF)
	structChanged = (rawVal & 0x00008000) != 0
	semChanged = (rawVal & 0x00004000) != 0
	infoBits = uint16(rawVal & 0x00003FFF)

	fullStr := status.Error()
	if idx := strings.LastIndex(fullStr, " (0x"); idx > 0 {
		mainPart := fullStr[:idx]
		if spPos := strings.Index(mainPart, ": "); spPos > 0 {
			symName = strings.TrimSpace(mainPart[spPos+2:])
		} else if dotPos := strings.Index(mainPart, ". "); dotPos > 0 {
			symName = strings.TrimSpace(mainPart[dotPos+2:])
		} else {
			symName = strings.TrimSpace(mainPart)
		}
	} else {
		symName = strings.TrimSpace(fullStr)
	}

	return
}
