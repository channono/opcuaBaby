package opc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	// "log"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

// isHexString reports whether s contains only hex digits (after any caller-provided normalization)
func isHexString(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

type DataChangeHandler interface {
	HandleDataChange(nodeID string, dv *ua.DataValue)
	HandleEvent(nodeID string, fields []*ua.Variant)
}

type Client struct {
	mu               sync.RWMutex
	Client           *opcua.Client
	endpoint         string
	sub              *opcua.Subscription
	dataChangeChan   chan *opcua.PublishNotificationData
	clientHandles    map[uint32]string
	monitoredItems   map[string]uint32
	clientHandleSeed uint32
	Handler          DataChangeHandler
}

type Subscription struct {
	nodeID       string
	parentClient *Client
}

func (s *Subscription) Close() error {
	return s.parentClient.UnmonitorItem(s.nodeID)
}

func NewClient(endpoint string, opts ...opcua.Option) (*Client, error) {
	cli, err := opcua.NewClient(endpoint, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		Client:         cli,
		endpoint:       endpoint,
		clientHandles:  make(map[uint32]string),
		monitoredItems: make(map[string]uint32),
	}, nil
}

func (c *Client) Connect(ctx context.Context) error {
	return c.Client.Connect(ctx)
}

func (c *Client) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Client == nil {
		return nil
	}

	if c.sub != nil {
		// Cancel the subscription; do not close dataChangeChan here.
		_ = c.sub.Cancel(context.Background())
	}

	err := c.Client.Close(ctx)

	c.Client = nil
	c.sub = nil
	c.dataChangeChan = nil
	c.clientHandles = make(map[uint32]string)
	c.monitoredItems = make(map[string]uint32)
	c.clientHandleSeed = 0

	return err
}

func (c *Client) MonitorItem(nodeID string) (*Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Client == nil {
		return nil, errors.New("client not connected")
	}
	if _, ok := c.monitoredItems[nodeID]; ok {
		return nil, fmt.Errorf("nodeID %s is already monitored", nodeID)
	}

	if c.sub == nil {
		c.dataChangeChan = make(chan *opcua.PublishNotificationData, 100)
		sub, err := c.Client.Subscribe(context.Background(), &opcua.SubscriptionParameters{
			Interval: 1000 * time.Millisecond,
		}, c.dataChangeChan)
		if err != nil {
			return nil, err
		}
		c.sub = sub
		go c.handleDataChanges()
	}

	id, err := ua.ParseNodeID(nodeID)
	if err != nil {
		return nil, err
	}

	handle := atomic.AddUint32(&c.clientHandleSeed, 1)
	req := opcua.NewMonitoredItemCreateRequestWithDefaults(id, ua.AttributeIDValue, handle)
	req.RequestedParameters.SamplingInterval = 500.0
	res, err := c.sub.Monitor(context.Background(), ua.TimestampsToReturnBoth, req)
	if err != nil {
		return nil, err
	}
	if res.Results[0].StatusCode != ua.StatusOK {
		return nil, fmt.Errorf("failed to monitor item: %s", res.Results[0].StatusCode)
	}

	c.clientHandles[handle] = nodeID
	c.monitoredItems[nodeID] = handle

	return &Subscription{nodeID: nodeID, parentClient: c}, nil
}

// MonitorMultiple 批量为一组 NodeIDs 创建 MonitoredItems，返回一个 Subscription
func (c *Client) MonitorMultiple(nodeIDs []string, parentNodeID string) (*Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Client == nil {
		return nil, errors.New("client not connected")
	}
	if c.sub == nil {
		c.dataChangeChan = make(chan *opcua.PublishNotificationData, 100)
		sub, err := c.Client.Subscribe(context.Background(), &opcua.SubscriptionParameters{
			Interval: 1000 * time.Millisecond,
		}, c.dataChangeChan)
		if err != nil {
			return nil, err
		}
		c.sub = sub
		go c.handleDataChanges()
	}

	for _, nid := range nodeIDs {
		id, err := ua.ParseNodeID(nid)
		if err != nil {
			return nil, err
		}
		handle := atomic.AddUint32(&c.clientHandleSeed, 1)
		req := opcua.NewMonitoredItemCreateRequestWithDefaults(id, ua.AttributeIDValue, handle)
		req.RequestedParameters.SamplingInterval = 500.0
		res, err := c.sub.Monitor(context.Background(), ua.TimestampsToReturnBoth, req)
		if err != nil {
			return nil, err
		}
		if res == nil || len(res.Results) == 0 || res.Results[0] == nil {
			return nil, fmt.Errorf("failed to monitor item: empty response for %s", nid)
		}
		if res.Results[0].StatusCode != ua.StatusOK {
			return nil, fmt.Errorf("failed to monitor item %s: %s", nid, res.Results[0].StatusCode)
		}

		c.clientHandles[handle] = nid
		c.monitoredItems[nid] = handle
	}

	return &Subscription{nodeID: parentNodeID, parentClient: c}, nil
}

// MonitorEvents 订阅指定 NodeID 的事件通知。
// nodeID 应该是具有 EventNotifier 属性的节点，例如服务器对象 (i=2253)。
func normalize61850SNodeID(s string) string {
	// Some servers expose IEC 61850 string NodeIds where S= should be lowercase 's='
	// Normalize common variants: ns=2:S=foo -> ns=2;s=foo
	re := regexp.MustCompile(`^ns=(\d+):S=`)
	return re.ReplaceAllString(s, "ns=$1;s=")
}

func (c *Client) MonitorEvents(nodeID string) (*Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Client == nil {
		return nil, errors.New("client not connected")
	}
	// 检查是否已经订阅了该事件源
	if _, ok := c.monitoredItems[nodeID]; ok {
		return nil, fmt.Errorf("nodeID %s is already monitored for events", nodeID)
	}

	// 如果还没有订阅，则创建新的订阅
	if c.sub == nil {
		c.dataChangeChan = make(chan *opcua.PublishNotificationData, 100)
		sub, err := c.Client.Subscribe(context.Background(), &opcua.SubscriptionParameters{
			Interval: 1000 * time.Millisecond, // 发布间隔，事件通知通过此间隔发送
		}, c.dataChangeChan)
		if err != nil {
			return nil, err
		}
		c.sub = sub
		go c.handleDataChanges()
	}

	// Normalize common IEC 61850 NodeId uppercase S= to s=
	nodeID = normalize61850SNodeID(strings.TrimSpace(nodeID))
	id, err := ua.ParseNodeID(nodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid node id: %w", err)
	}
	// Validate that the target node is an event source object with SubscribeToEvents capability
	nc, err := c.ReadNodeClass(context.Background(), id)
	if err != nil {
		return nil, fmt.Errorf("failed to read NodeClass for %s: %v", nodeID, err)
	}
	if nc != ua.NodeClassObject {
		return nil, fmt.Errorf("node %s is not an Object (NodeClass=%v); select the ReportControl object to subscribe", nodeID, nc)
	}
	// EventNotifier attribute must have SubscribeToEvents bit (0x1)
	enVals, err := c.ReadAttributes(context.Background(), nodeID, ua.AttributeIDEventNotifier)
	if err != nil {
		return nil, fmt.Errorf("failed to read EventNotifier for %s: %v", nodeID, err)
	}
	if len(enVals) == 0 || enVals[0] == nil || enVals[0].Value == nil {
		return nil, fmt.Errorf("EventNotifier missing for %s", nodeID)
	}
	subBit := func(v interface{}) (bool, uint32) {
		switch t := v.(type) {
		case byte:
			return (uint32(t) & 0x1) != 0, uint32(t)
		case uint16:
			return (uint32(t) & 0x1) != 0, uint32(t)
		case uint32:
			return (t & 0x1) != 0, t
		case int32:
			return (uint32(t) & 0x1) != 0, uint32(t)
		default:
			return false, 0
		}
	}
	okSub, raw := subBit(enVals[0].Value.Value())
	if !okSub {
		return nil, fmt.Errorf("node %s EventNotifier=0x%x does not allow SubscribeToEvents; select the ReportControl object node", nodeID, raw)
	}

	// 定义事件过滤器：我们感兴趣的 BaseEventType 字段
	// For IEC 61850 RCBs, the event type is specific and does not contain standard fields like "Message".
	// An empty SelectClauses requests ALL fields of the event, which is the correct approach for custom event types.
	// Build a robust EventFilter with standard BaseEventType fields for maximum interoperability
	// Per spec, many servers accept SimpleAttributeOperand with nil TypeDefinitionID and a BrowsePath from BaseEventType
	mkOp := func(field string) *ua.SimpleAttributeOperand {
		return &ua.SimpleAttributeOperand{
			TypeDefinitionID: nil,
			BrowsePath:       []*ua.QualifiedName{{NamespaceIndex: 0, Name: field}},
			AttributeID:      ua.AttributeIDValue,
		}
	}
	// First try a minimal, highly compatible filter (EventId only) with explicit empty WhereClause
	minimal := &ua.EventFilter{SelectClauses: []*ua.SimpleAttributeOperand{mkOp("EventId")}, WhereClause: &ua.ContentFilter{Elements: []*ua.ContentFilterElement{}}}
	handle := atomic.AddUint32(&c.clientHandleSeed, 1)
	// 创建监控项请求，AttributeIDEventNotifier 用于事件订阅
	req := opcua.NewMonitoredItemCreateRequestWithDefaults(id, ua.AttributeIDEventNotifier, handle)
	req.RequestedParameters.Filter = ua.NewExtensionObject(minimal)
	req.RequestedParameters.SamplingInterval = 0.0 // 事件通常不需要采样

	res, err := c.sub.Monitor(context.Background(), ua.TimestampsToReturnBoth, req)
	if err != nil {
		return nil, err
	}
	if res == nil || len(res.Results) == 0 || res.Results[0] == nil {
		return nil, fmt.Errorf("failed to monitor events: empty response")
	}
	fmt.Printf("DEBUG: MonitorEvents Minimal Status: %s, Revised Sampling Interval: %f\n", res.Results[0].StatusCode, res.Results[0].RevisedSamplingInterval)

	if res.Results[0].StatusCode != ua.StatusOK {
		// Retry with a richer BaseEventType field set
		selectAll := []*ua.SimpleAttributeOperand{
			mkOp("EventId"),
			mkOp("EventType"),
			mkOp("SourceNode"),
			mkOp("SourceName"),
			mkOp("Time"),
			mkOp("ReceiveTime"),
			mkOp("Message"),
			mkOp("Severity"),
		}
		rich := &ua.EventFilter{SelectClauses: selectAll, WhereClause: &ua.ContentFilter{Elements: []*ua.ContentFilterElement{}}}
		req.RequestedParameters.Filter = ua.NewExtensionObject(rich)
		res, err = c.sub.Monitor(context.Background(), ua.TimestampsToReturnBoth, req)
		if err != nil {
			return nil, err
		}
		if res == nil || len(res.Results) == 0 || res.Results[0] == nil {
			return nil, fmt.Errorf("failed to monitor events (retry): empty response")
		}
		fmt.Printf("DEBUG: MonitorEvents Retry(rich) Status: %s\n", res.Results[0].StatusCode)
		if res.Results[0].StatusCode != ua.StatusOK {
			return nil, fmt.Errorf("failed to monitor events: %s", res.Results[0].StatusCode)
		}
	}

	c.clientHandles[handle] = nodeID
	c.monitoredItems[nodeID] = handle // 仍然使用 monitoredItems 跟踪，但需要区分数据和事件

	return &Subscription{nodeID: nodeID, parentClient: c}, nil
}

func (c *Client) WriteValue(ctx context.Context, nodeID string, value interface{}) error {
	c.mu.RLock()
	if c.Client == nil {
		c.mu.RUnlock()
		return errors.New("opc ua client is not connected")
	}
	cli := c.Client
	c.mu.RUnlock()

	id, err := ua.ParseNodeID(nodeID)
	if err != nil {
		return fmt.Errorf("invalid node id: %w", err)
	}

	// If value is a string that looks like hex (e.g., "45dc" or "0x45 dc"),
	// convert it to []byte so we write exact bytes instead of ASCII characters.
	if s, ok := value.(string); ok {
		hs := strings.TrimSpace(s)
		hs = strings.ToLower(strings.ReplaceAll(hs, " ", ""))
		hs = strings.TrimPrefix(hs, "0x")
		if len(hs)%2 == 0 && isHexString(hs) {
			if b, decErr := hex.DecodeString(hs); decErr == nil {
				value = b
			}
		}
	}

	v, err := ua.NewVariant(value)
	if err != nil {
		return fmt.Errorf("failed to create variant: %w", err)
	}

	req := &ua.WriteRequest{
		NodesToWrite: []*ua.WriteValue{
			{
				NodeID:      id,
				AttributeID: ua.AttributeIDValue,
				Value: &ua.DataValue{
					EncodingMask: ua.DataValueValue,
					Value:        v,
				},
			},
		},
	}

	resp, err := cli.Write(ctx, req)
	if err != nil {
		return err
	}

	if len(resp.Results) > 0 && resp.Results[0] != ua.StatusOK {
		return fmt.Errorf("write failed with status: %s", resp.Results[0])
	}

	return nil
}

// CallMethod invokes a method on an object node. objectNodeID is the object that owns the method.
func (c *Client) CallMethod(objectNodeID string, methodNodeID string, inputArgs ...interface{}) ([]*ua.Variant, error) {
	c.mu.RLock()
	if c.Client == nil {
		c.mu.RUnlock()
		return nil, errors.New("client not connected")
	}
	cli := c.Client
	c.mu.RUnlock()

	obj, err := ua.ParseNodeID(objectNodeID)
	if err != nil {
		return nil, err
	}
	mth, err := ua.ParseNodeID(methodNodeID)
	if err != nil {
		return nil, err
	}

	// Build input argument Variants
	in := make([]*ua.Variant, 0, len(inputArgs))
	for _, a := range inputArgs {
		v, verr := ua.NewVariant(a)
		if verr != nil {
			return nil, verr
		}
		in = append(in, v)
	}

	req := &ua.CallMethodRequest{ObjectID: obj, MethodID: mth, InputArguments: in}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := cli.Call(ctx, req)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("empty call result")
	}
	if res.StatusCode != ua.StatusOK {
		return nil, fmt.Errorf("method call failed: %s", res.StatusCode)
	}
	return res.OutputArguments, nil
}

// //
func (c *Client) ReadAttributes(ctx context.Context, nodeID string, attributeIDs ...ua.AttributeID) ([]*ua.DataValue, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.Client == nil {
		return nil, errors.New("client not connected")
	}

	id, err := ua.ParseNodeID(nodeID)
	if err != nil {
		return nil, err
	}

	nodesToRead := make([]*ua.ReadValueID, len(attributeIDs))
	for i, attrID := range attributeIDs {
		nodesToRead[i] = &ua.ReadValueID{NodeID: id, AttributeID: attrID}
	}

	req := &ua.ReadRequest{NodesToRead: nodesToRead}
	resp, err := c.Client.Read(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Results, nil
}

func (c *Client) Browse(ctx context.Context, nodeID *ua.NodeID) ([]*ua.ReferenceDescription, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.Client == nil {
		return nil, errors.New("client not connected")
	}

	req := &ua.BrowseRequest{
		NodesToBrowse: []*ua.BrowseDescription{
			{
				NodeID:          nodeID,
				BrowseDirection: ua.BrowseDirectionForward,
				ReferenceTypeID: ua.NewNumericNodeID(0, 33), // HierarchicalReferences
				IncludeSubtypes: true,
				NodeClassMask:   uint32(ua.NodeClassAll),
				ResultMask:      uint32(ua.BrowseResultMaskAll),
			},
		},
		RequestedMaxReferencesPerNode: 1000,
	}

	resp, err := c.Client.Browse(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Results) > 0 {
		return resp.Results[0].References, nil
	}
	return nil, nil
}

func (c *Client) handleDataChanges() {
	for ntf := range c.dataChangeChan {
		if ntf == nil {
			continue
		}
		if ntf.Error != nil {
			fmt.Printf("Subscription error: %v\n", ntf.Error)
			continue
		}

		// Use a type switch to handle different notification types
		switch data := ntf.Value.(type) {
		case *ua.DataChangeNotification:
			for _, item := range data.MonitoredItems {
				if item == nil || item.Value == nil {
					continue
				}
				c.mu.RLock()
				nodeID, ok := c.clientHandles[item.ClientHandle]
				handler := c.Handler
				c.mu.RUnlock()
				if ok && handler != nil {
					handler.HandleDataChange(nodeID, item.Value)
				}
			}
		case *ua.EventNotificationList:
			// DEBUG: Print the raw event notification list
			fmt.Printf("DEBUG: Received EventNotificationList: %+v\n", data)

			for _, item := range data.Events {
				if item == nil || item.EventFields == nil {
					continue
				}
				c.mu.RLock()
				nodeID, ok := c.clientHandles[item.ClientHandle]
				handler := c.Handler
				c.mu.RUnlock()
				if ok && handler != nil {
					handler.HandleEvent(nodeID, item.EventFields)
				}
			}
		}
	}
}

func (c *Client) ReadNodeClass(ctx context.Context, nodeID *ua.NodeID) (ua.NodeClass, error) {
	results, err := c.ReadAttributes(ctx, nodeID.String(), ua.AttributeIDNodeClass)
	if err != nil {
		return 0, err
	}
	if len(results) == 0 || results[0].Value == nil {
		return 0, errors.New("attribute read incomplete")
	}
	if v, ok := results[0].Value.Value().(int32); ok {
		return ua.NodeClass(v), nil
	}
	return 0, fmt.Errorf("unexpected type for NodeClass: %T", results[0].Value.Value())
}

func (c *Client) UnmonitorItem(nodeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	handle, ok := c.monitoredItems[nodeID]
	if !ok {
		return fmt.Errorf("nodeID %s is not monitored", nodeID)
	}

	if c.sub != nil {
		_, _ = c.sub.Unmonitor(context.Background(), handle)
	}

	delete(c.monitoredItems, nodeID)
	delete(c.clientHandles, handle)

	if len(c.monitoredItems) == 0 && c.sub != nil {
		_ = c.sub.Cancel(context.Background())
		c.sub = nil
		c.dataChangeChan = nil
	}

	return nil
}
