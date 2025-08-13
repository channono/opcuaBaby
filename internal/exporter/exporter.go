package exporter

import (
    "context"
    "encoding/csv"
    "encoding/json"
    "fmt"
    "opcuababy/internal/opc"
    "os"
    "strings"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/xuri/excelize/v2"
)

// ExportNode represents a node in the address space for export purposes.
type ExportNode struct {
	Name        string        `json:"name"`
	NodeID      string        `json:"nodeId"`
	NodeClass   string        `json:"nodeClass"`
	DataType    string        `json:"dataType,omitempty"`
	AccessLevel string        `json:"accessLevel,omitempty"`
	Description string        `json:"description,omitempty"`
	Value       string        `json:"value,omitempty"`
	Children    []*ExportNode `json:"children,omitempty"`
}

// ExportToCSV exports the full address space (starting from rootNodeID) to a CSV file.
func (e *Exporter) ExportToCSV(ctx context.Context, rootNodeID, filePath string) error {
    visited := make(map[string]struct{})
    rootNode, err := e.buildTree(ctx, rootNodeID, visited)
    if err != nil {
        return fmt.Errorf("failed to build address space tree: %w", err)
    }

    f, err := os.Create(filePath)
    if err != nil {
        return err
    }
    defer f.Close()

    w := csv.NewWriter(f)
    defer w.Flush()

    _ = w.Write([]string{"Level", "Name", "NodeID", "NodeClass", "DataType", "AccessLevel", "Description", "Value"})

    // Iterative stack to avoid deep recursion
    type frame struct { node *ExportNode; level int }
    stack := []frame{{node: rootNode, level: 0}}
    for len(stack) > 0 {
        fr := stack[len(stack)-1]
        stack = stack[:len(stack)-1]
        _ = w.Write([]string{
            fmt.Sprintf("%d", fr.level), fr.node.Name, fr.node.NodeID, fr.node.NodeClass,
            fr.node.DataType, fr.node.AccessLevel, fr.node.Description, fr.node.Value,
        })
        // push children in reverse to keep natural order
        for i := len(fr.node.Children) - 1; i >= 0; i-- {
            stack = append(stack, frame{node: fr.node.Children[i], level: fr.level + 1})
        }
    }
    return nil
}

// Exporter handles the logic for exporting the address space.
type Exporter struct {
	client *opc.Client
}

// New creates a new Exporter.
func New(client *opc.Client) *Exporter {
	return &Exporter{client: client}
}

// ExportToJSON exports the full address space starting from rootNodeID to a JSON file.
func (e *Exporter) ExportToJSON(ctx context.Context, rootNodeID, filePath string) error {
    visited := make(map[string]struct{})
    rootNode, err := e.buildTree(ctx, rootNodeID, visited)
	if err != nil {
		return fmt.Errorf("failed to build address space tree: %w", err)
	}

	data, err := json.MarshalIndent(rootNode, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tree to JSON: %w", err)
	}

	return os.WriteFile(filePath, data, 0644)
}

// ExportToExcel exports the full address space starting from rootNodeID to an Excel file.
func (e *Exporter) ExportToExcel(ctx context.Context, rootNodeID, filePath string) error {
    visited := make(map[string]struct{})
    rootNode, err := e.buildTree(ctx, rootNodeID, visited)
	if err != nil {
		return fmt.Errorf("failed to build address space tree: %w", err)
	}

	f := excelize.NewFile()
	sheetName := "OPC UA Address Space"
	_, err = f.NewSheet(sheetName)
	if err != nil {
		return err
	}
	f.DeleteSheet("Sheet1")

	headers := []string{"Level", "Name", "NodeID", "NodeClass", "DataType", "AccessLevel", "Description", "Value"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, h)
	}

	row := 2
	e.writeExcelRow(f, sheetName, rootNode, 0, &row)

	return f.SaveAs(filePath)
}

// buildTree recursively browses the address space from the given nodeID and builds a tree.
// visited ensures we don't loop forever if the server exposes cyclic references.
func (e *Exporter) buildTree(ctx context.Context, nodeID string, visited map[string]struct{}) (*ExportNode, error) {
    // Cycle protection
    if _, ok := visited[nodeID]; ok {
        // already visited: don't expand; try to keep a human-readable name
        attrs, _ := e.readAttributes(ctx, nodeID)
        name := nodeID
        if attrs != nil && attrs.Name != "" {
            name = attrs.Name
        }
        return &ExportNode{ Name: name, NodeID: nodeID }, nil
    }

    attrs, err := e.readAttributes(ctx, nodeID)
    if err != nil {
        return nil, err
    }

    exportNode := &ExportNode{
        Name:        attrs.Name,
        NodeID:      attrs.NodeID,
        NodeClass:   attrs.NodeClass,
        DataType:    attrs.DataType,
        AccessLevel: attrs.AccessLevel,
        Description: attrs.Description,
        Value:       attrs.Value,
        Children:    []*ExportNode{},
    }
    // mark visited after we know the real NodeID
    visited[exportNode.NodeID] = struct{}{}

    // Only browse children if the node is not a variable (i.e., it's an object or view)
    if exportNode.NodeClass != ua.NodeClassVariable.String() {
        browseCtx, cancel := context.WithTimeout(ctx, 30*time.Second) // Timeout for each browse call
        defer cancel()
        refs, err := e.client.Browse(browseCtx, ua.MustParseNodeID(nodeID))
        if err != nil {
            // Log the error but continue, as some nodes might not be browsable
            fmt.Printf("could not browse node %s: %v\n", nodeID, err)
        } else {
            for _, ref := range refs {
                // Check for context cancellation before recursing
                if ctx.Err() != nil {
                    return nil, ctx.Err()
                }
                // Skip if we've seen this NodeID to avoid cycles
                cid := ref.NodeID.String()
                if _, ok := visited[cid]; ok {
                    continue
                }
                childNode, err := e.buildTree(ctx, cid, visited)
                if err != nil {
                    fmt.Printf("Skipping child node %s due to error: %v\n", ref.NodeID.String(), err)
                    continue
                }
                exportNode.Children = append(exportNode.Children, childNode)
            }
        }
    }

    return exportNode, nil
}

// readAttributes reads all relevant attributes for a given node.
func (e *Exporter) readAttributes(ctx context.Context, nodeID string) (*ExportNode, error) {
    attrsToRead := []ua.AttributeID{
        ua.AttributeIDNodeID,
        ua.AttributeIDNodeClass,
        ua.AttributeIDDisplayName,
        ua.AttributeIDDescription,
        ua.AttributeIDAccessLevel,
        ua.AttributeIDDataType,
        ua.AttributeIDValue,
    }

	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	results, err := e.client.ReadAttributes(readCtx, nodeID, attrsToRead...)
	if err != nil {
		return nil, err
	}

	attrs := &ExportNode{NodeID: nodeID}
	for i, res := range results {
		if res.Status != ua.StatusOK || res.Value == nil {
			continue
		}
		switch attrsToRead[i] {
		case ua.AttributeIDNodeClass:
			if val, ok := res.Value.Value().(int32); ok {
				attrs.NodeClass = ua.NodeClass(val).String()
			}
		case ua.AttributeIDDisplayName:
			if val, ok := res.Value.Value().(ua.LocalizedText); ok {
				attrs.Name = val.Text
			}
		case ua.AttributeIDDescription:
			if val, ok := res.Value.Value().(ua.LocalizedText); ok {
				attrs.Description = val.Text
			}
		case ua.AttributeIDAccessLevel:
			var levelValue uint32
			switch v := res.Value.Value().(type) {
			case uint8:
				levelValue = uint32(v)
			case uint32:
				levelValue = v
			}
			if levelValue > 0 {
				attrs.AccessLevel = formatAccessLevel(ua.AccessLevelType(levelValue))
			}
		case ua.AttributeIDDataType:
			if dt, ok := res.Value.Value().(*ua.NodeID); ok {
				attrs.DataType = builtinTypeName(dt)
			}
		case ua.AttributeIDValue:
			attrs.Value = fmt.Sprintf("%v", res.Value.Value())
		}
	}
	if attrs.Name == "" {
		attrs.Name = nodeID
	}
	return attrs, nil
}

func (e *Exporter) writeExcelRow(f *excelize.File, sheetName string, node *ExportNode, level int, row *int) {
	// Write current node
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", *row), level)
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", *row), node.Name)
	f.SetCellValue(sheetName, fmt.Sprintf("C%d", *row), node.NodeID)
	f.SetCellValue(sheetName, fmt.Sprintf("D%d", *row), node.NodeClass)
	f.SetCellValue(sheetName, fmt.Sprintf("E%d", *row), node.DataType)
	f.SetCellValue(sheetName, fmt.Sprintf("F%d", *row), node.AccessLevel)
	f.SetCellValue(sheetName, fmt.Sprintf("G%d", *row), node.Description)
	f.SetCellValue(sheetName, fmt.Sprintf("H%d", *row), node.Value)
	(*row)++

	// Write children
	for _, child := range node.Children {
		e.writeExcelRow(f, sheetName, child, level+1, row)
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
	return strings.Join(parts, ", ")
}

func builtinTypeName(id *ua.NodeID) string {
	if id == nil {
		return ""
	}
	if id.Namespace() != 0 {
		return id.String()
	}
	var typeNames = map[uint32]string{
		1: "Boolean", 2: "SByte", 3: "Byte", 4: "Int16", 5: "UInt16", 6: "Int32", 7: "UInt32", 8: "Int64", 9: "UInt64",
		10: "Float", 11: "Double", 12: "String", 13: "DateTime", 14: "Guid", 15: "ByteString", 16: "XmlElement",
		17: "NodeId", 18: "ExpandedNodeId", 19: "StatusCode", 20: "QualifiedName", 21: "LocalizedText",
		22: "ExtensionObject", 23: "DataValue", 24: "Variant", 25: "DiagnosticInfo",
	}
	if name, ok := typeNames[id.IntID()]; ok {
		return name
	}
	return id.String()
}
