package opc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
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

///////

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

////
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
		dcn, ok := ntf.Value.(*ua.DataChangeNotification)
		if !ok || dcn == nil {
			continue
		}
		for _, item := range dcn.MonitoredItems {
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