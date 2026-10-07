package cassandra

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/graemenewlands/go-ops5-apps/apps/cassandra/data"
	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
)

//go:embed rules/cassandra.ops
var CassandraRulesSource string

// MessageEvent represents an in-flight, delivered, or dropped network message.
type MessageEvent struct {
	Kind       string `json:"kind"` // request, response, counted_local, etc.
	FromNode   string `json:"fromNode"`
	ToNode     string `json:"toNode"`
	IsLocal    bool   `json:"isLocal"`
	Status     string `json:"status"` // in_flight, delivered, dropped
	PayloadVal string `json:"payloadVal"`
	PayloadTS  int64  `json:"payloadTs"`
}

// HintEvent represents a hinted handoff written to coordinator disk for a down replica.
type HintEvent struct {
	Coordinator string `json:"coordinator"`
	TargetNode  string `json:"targetNode"`
	Value       string `json:"value"`
	Timestamp   int64  `json:"timestamp"`
}

// ReadRepairEvent represents a background read-repair update to an out-of-date replica.
type ReadRepairEvent struct {
	TargetNode  string `json:"targetNode"`
	RepairedVal string `json:"repairedVal"`
	RepairedTS  int64  `json:"repairedTs"`
}

// LiveTallyResult summarizes live replica counts discovered before query dispatch.
type LiveTallyResult struct {
	LocalAlive  int `json:"localAlive"`
	RemoteAlive int `json:"remoteAlive"`
	TotalAlive  int `json:"totalAlive"`
}

// AckResult summarizes acknowledgments accumulated by the coordinator.
type AckResult struct {
	LocalAcks  int `json:"localAcks"`
	RemoteAcks int `json:"remoteAcks"`
	TotalAcks  int `json:"totalAcks"`
}

// EngineStats records OPS5 execution metrics.
type EngineStats struct {
	CycleCount int     `json:"cycleCount"`
	ElapsedMs  float64 `json:"elapsedMs"`
	WMECounter int     `json:"wmeCount"`
}

// QueryResult represents the outcome of a Cassandra query simulation.
type QueryResult struct {
	QueryID          string            `json:"queryId"`
	Success          bool              `json:"success"`
	ErrorReason      string            `json:"errorReason,omitempty"`
	QueryType        string            `json:"queryType"` // read, write
	TargetKey        string            `json:"targetKey"`
	ConsistencyLevel string            `json:"consistencyLevel"`
	Coordinator      string            `json:"coordinator"`
	LocalDC          string            `json:"localDc"`
	LiveTally        LiveTallyResult   `json:"liveTally"`
	AckResult        AckResult         `json:"ackResult"`
	ResolvedValue    string            `json:"resolvedValue"`
	HighestTimestamp int64             `json:"highestTimestamp"`
	Messages         []MessageEvent    `json:"messages"`
	HintedHandoffs   []HintEvent       `json:"hintedHandoffs"`
	ReadRepairs      []ReadRepairEvent `json:"readRepairs"`
	Stats            EngineStats       `json:"stats"`
}

// CassandraEngine wraps the OPS5 rule engine and cluster state.
type CassandraEngine struct {
	mu            sync.RWMutex
	cluster       *data.ClusterConfig
	clientDC      string
	coordinatorID string
	wanConnected  bool // True = WAN operational, False = Network Partition
	queryCounter  int64
	parsedRules   []*model.Rule
}

// NewCassandraEngine creates a newly initialized Cassandra engine.
func NewCassandraEngine(cfg *data.ClusterConfig) (*CassandraEngine, error) {
	if cfg == nil {
		cfg = data.DefaultCluster()
	}

	rules, err := parser.ParseRules(CassandraRulesSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cassandra rules: %w", err)
	}

	ce := &CassandraEngine{
		cluster:       cfg,
		clientDC:      "dc1",
		coordinatorID: "dc1-n1",
		wanConnected:  true,
		parsedRules:   rules,
	}

	return ce, nil
}

// ClusterConfig returns the current cluster configuration.
func (ce *CassandraEngine) ClusterConfig() *data.ClusterConfig {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.cluster
}

// WANConnected returns true if the inter-datacenter WAN link is operational.
func (ce *CassandraEngine) WANConnected() bool {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.wanConnected
}

// SetWANConnected explicitly sets the inter-datacenter WAN link state.
func (ce *CassandraEngine) SetWANConnected(connected bool) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.wanConnected = connected
}

// ToggleWAN toggles the inter-datacenter WAN link between operational and severed.
func (ce *CassandraEngine) ToggleWAN() bool {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.wanConnected = !ce.wanConnected
	return ce.wanConnected
}

// ClientDC returns the connected client's datacenter.
func (ce *CassandraEngine) ClientDC() string {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.clientDC
}

// CoordinatorID returns the active coordinator node ID.
func (ce *CassandraEngine) CoordinatorID() string {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.coordinatorID
}

// SetClientDC sets the client's connected datacenter and selects a default coordinator in that DC if needed.
func (ce *CassandraEngine) SetClientDC(dcID string) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	found := false
	for _, dc := range ce.cluster.DCs {
		if dc.ID == dcID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("datacenter %s not found", dcID)
	}

	ce.clientDC = dcID

	// Check if current coordinator belongs to this DC
	coordNode := ce.findNode(ce.coordinatorID)
	if coordNode == nil || coordNode.DC != dcID {
		// Pick first node of this DC
		for _, dc := range ce.cluster.DCs {
			if dc.ID == dcID && len(dc.Nodes) > 0 {
				ce.coordinatorID = dc.Nodes[0].ID
				break
			}
		}
	}

	return nil
}

// SetCoordinator explicitly selects a node as Coordinator and updates clientDC accordingly.
func (ce *CassandraEngine) SetCoordinator(nodeID string) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	node := ce.findNode(nodeID)
	if node == nil {
		return fmt.Errorf("node %s not found", nodeID)
	}

	ce.coordinatorID = node.ID
	ce.clientDC = node.DC
	return nil
}

// PullPlug sets a node's state directly to DS (Down, Stopped).
func (ce *CassandraEngine) PullPlug(nodeID string) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	node := ce.findNode(nodeID)
	if node == nil {
		return fmt.Errorf("node %s not found", nodeID)
	}

	node.Health = data.HealthDown
	node.Membership = data.MembershipStopped
	return nil
}

// CycleNodeState cycles a node through DS -> UJ -> UN.
// (DJ is explicitly not a valid state enum).
func (ce *CassandraEngine) CycleNodeState(nodeID string) (string, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	node := ce.findNode(nodeID)
	if node == nil {
		return "", fmt.Errorf("node %s not found", nodeID)
	}

	curr := node.StateCode()
	switch curr {
	case "DS", "DN":
		node.Health = data.HealthUp
		node.Membership = data.MembershipJoining
	case "UJ":
		node.Health = data.HealthUp
		node.Membership = data.MembershipNormal
	case "UN":
		node.Health = data.HealthDown
		node.Membership = data.MembershipStopped
	default:
		node.Health = data.HealthDown
		node.Membership = data.MembershipStopped
	}

	return node.StateCode(), nil
}

// SetNodeState sets health and membership explicitly, validating that DJ is rejected.
func (ce *CassandraEngine) SetNodeState(nodeID string, health data.NodeHealth, membership data.NodeMembership) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if health == data.HealthDown && membership == data.MembershipJoining {
		return fmt.Errorf("invalid state enum: DJ is not permitted in Cassandra lifecycle")
	}

	node := ce.findNode(nodeID)
	if node == nil {
		return fmt.Errorf("node %s not found", nodeID)
	}

	node.Health = health
	node.Membership = membership
	return nil
}

// ResetCluster resets the cluster to default clean state.
func (ce *CassandraEngine) ResetCluster() {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	ce.cluster = data.DefaultCluster()
	ce.clientDC = "dc1"
	ce.coordinatorID = "dc1-n1"
	ce.wanConnected = true
}

// ExecuteQuery executes a read or write Cassandra query using OPS5 forward-chaining rules.
// If optCoord is provided, it uses that coordinator and its datacenter without mutating the engine's default clientDC/coordinatorID.
func (ce *CassandraEngine) ExecuteQuery(qType string, cl data.ConsistencyLevel, key, writeVal string, optCoord ...string) (*QueryResult, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	ce.queryCounter++
	queryID := fmt.Sprintf("q-%d", ce.queryCounter)

	// Validate query parameters
	qType = strings.ToLower(qType)
	if qType != "read" && qType != "write" {
		qType = "read"
	}
	if key == "" {
		key = ce.cluster.DefaultDataset
	}
	if writeVal == "" {
		writeVal = fmt.Sprintf("val_ts_%d", time.Now().UnixMilli())
	}
	writeTS := time.Now().UnixMilli()

	activeCoord := ce.coordinatorID
	activeDC := ce.clientDC
	if len(optCoord) > 0 && optCoord[0] != "" {
		if node := ce.findNode(optCoord[0]); node != nil {
			activeCoord = node.ID
			activeDC = node.DC
		}
	}

	// Instantiate fresh OPS5 engine for this query run
	eng := engine.New()
	eng.SetStrategy(conflict.StrategyLEX)
	_ = eng.SetWatchLevel(0)

	for _, r := range ce.parsedRules {
		eng.AddRule(r)
	}

	// 1. Assert cluster metadata
	wanVal := int64(1)
	if !ce.wanConnected {
		wanVal = 0
	}
	eng.Make("cluster_meta", map[string]model.Value{
		"total_replicas": model.NewInt(int64(ce.cluster.ReplicasPerDC * len(ce.cluster.DCs))),
		"local_replicas": model.NewInt(int64(ce.cluster.ReplicasPerDC)),
		"wan_connected":  model.NewInt(wanVal),
	})

	// 2. Assert all nodes across both rings
	for _, dc := range ce.cluster.DCs {
		for _, n := range dc.Nodes {
			isRepVal := int64(0)
			if n.IsReplica {
				isRepVal = 1
			}
			valStr := n.Value
			if valStr == "" {
				valStr = "none"
			}
			eng.Make("node", map[string]model.Value{
				"id":         model.NewSymbol(n.ID),
				"dc":         model.NewSymbol(n.DC),
				"ring_pos":   model.NewInt(int64(n.RingPos)),
				"health":     model.NewSymbol(string(n.Health)),
				"membership": model.NewSymbol(string(n.Membership)),
				"is_replica": model.NewInt(isRepVal),
				"value":      model.NewSymbol(valStr),
				"timestamp":  model.NewInt(n.Timestamp),
			})
		}
	}

	// 3. Assert client connection
	eng.Make("client_connection", map[string]model.Value{
		"dc":             model.NewSymbol(activeDC),
		"coordinator_id": model.NewSymbol(activeCoord),
	})

	// 4. Assert Query initiation WME
	eng.Make("query", map[string]model.Value{
		"id":           model.NewSymbol(queryID),
		"type":         model.NewSymbol(qType),
		"key":          model.NewSymbol(key),
		"cl":           model.NewSymbol(string(cl)),
		"write_val":    model.NewSymbol(writeVal),
		"write_ts":     model.NewInt(writeTS),
		"phase":        model.NewSymbol("init"),
		"coordinator":  model.NewSymbol(activeCoord),
		"local_dc":     model.NewSymbol(activeDC),
		"status":       model.NewSymbol("pending"),
		"error_reason": model.NewSymbol("none"),
	})

	// Run OPS5 engine to quiescence
	startTime := time.Now()
	initialCycles := eng.CycleCount()

	_, err := eng.Run(0)
	if err != nil {
		return nil, fmt.Errorf("error running ops5 cassandra rules: %w", err)
	}

	elapsed := time.Since(startTime)
	cyclesFired := eng.CycleCount() - initialCycles

	// Extract results from working memory
	res := &QueryResult{
		QueryID:          queryID,
		QueryType:        qType,
		TargetKey:        key,
		ConsistencyLevel: string(cl),
		Coordinator:      activeCoord,
		LocalDC:          activeDC,
		Messages:         make([]MessageEvent, 0),
		HintedHandoffs:   make([]HintEvent, 0),
		ReadRepairs:      make([]ReadRepairEvent, 0),
		Stats: EngineStats{
			CycleCount: cyclesFired,
			ElapsedMs:  float64(elapsed.Microseconds()) / 1000.0,
			WMECounter: eng.WorkingMemory().Count(),
		},
	}

	// Query status and error
	queryWMEs := eng.WorkingMemory().FindByClass("query")
	if len(queryWMEs) > 0 {
		qw := queryWMEs[0]
		if stVal, ok := qw.Get("status"); ok {
			res.Success = (fmt.Sprintf("%v", stVal.Raw()) == "success")
		}
		if errVal, ok := qw.Get("error_reason"); ok {
			errStr := fmt.Sprintf("%v", errVal.Raw())
			if errStr != "none" {
				res.ErrorReason = errStr
			}
		}
	}

	// Live tally
	tallyWMEs := eng.WorkingMemory().FindByClass("live_tally")
	if len(tallyWMEs) > 0 {
		tw := tallyWMEs[0]
		if loc, ok := tw.Get("local_alive"); ok {
			res.LiveTally.LocalAlive = int(loc.Raw().(int64))
		}
		if rem, ok := tw.Get("remote_alive"); ok {
			res.LiveTally.RemoteAlive = int(rem.Raw().(int64))
		}
		if tot, ok := tw.Get("total_alive"); ok {
			res.LiveTally.TotalAlive = int(tot.Raw().(int64))
		}
	}

	// Ack accumulator
	accWMEs := eng.WorkingMemory().FindByClass("ack_accumulator")
	if len(accWMEs) > 0 {
		aw := accWMEs[0]
		if loc, ok := aw.Get("local_acks"); ok {
			res.AckResult.LocalAcks = int(loc.Raw().(int64))
		}
		if rem, ok := aw.Get("remote_acks"); ok {
			res.AckResult.RemoteAcks = int(rem.Raw().(int64))
		}
		if tot, ok := aw.Get("total_acks"); ok {
			res.AckResult.TotalAcks = int(tot.Raw().(int64))
		}
		if hi, ok := aw.Get("highest_ts"); ok {
			res.HighestTimestamp = hi.Raw().(int64)
		}
		if rval, ok := aw.Get("resolved_val"); ok {
			res.ResolvedValue = fmt.Sprintf("%v", rval.Raw())
		}
	}

	// Messages (requests & responses)
	msgWMEs := eng.WorkingMemory().FindByClass("message")
	for _, mw := range msgWMEs {
		kindVal, _ := mw.Get("kind")
		kindStr := fmt.Sprintf("%v", kindVal.Raw())
		if kindStr != "request" && kindStr != "response" {
			continue // filter internal counting tokens
		}

		fromVal, _ := mw.Get("from_node")
		toVal, _ := mw.Get("to_node")
		stVal, _ := mw.Get("status")
		pVal, _ := mw.Get("payload_val")

		isLoc := false
		if isLocVal, ok := mw.Get("is_local"); ok {
			if v, ok := isLocVal.Raw().(int64); ok && v == 1 {
				isLoc = true
			}
		}

		var pts int64 = 0
		if pTSVal, ok := mw.Get("payload_ts"); ok {
			if v, ok := pTSVal.Raw().(int64); ok {
				pts = v
			}
		}

		res.Messages = append(res.Messages, MessageEvent{
			Kind:       kindStr,
			FromNode:   fmt.Sprintf("%v", fromVal.Raw()),
			ToNode:     fmt.Sprintf("%v", toVal.Raw()),
			IsLocal:    isLoc,
			Status:     fmt.Sprintf("%v", stVal.Raw()),
			PayloadVal: fmt.Sprintf("%v", pVal.Raw()),
			PayloadTS:  pts,
		})
	}

	// Hinted handoffs
	hintWMEs := eng.WorkingMemory().FindByClass("hinted_handoff")
	for _, hw := range hintWMEs {
		coordVal, _ := hw.Get("coordinator")
		targetVal, _ := hw.Get("target_node")
		vVal, _ := hw.Get("value")

		var ts int64 = 0
		if tsVal, ok := hw.Get("timestamp"); ok {
			if v, ok := tsVal.Raw().(int64); ok {
				ts = v
			}
		}

		res.HintedHandoffs = append(res.HintedHandoffs, HintEvent{
			Coordinator: fmt.Sprintf("%v", coordVal.Raw()),
			TargetNode:  fmt.Sprintf("%v", targetVal.Raw()),
			Value:       fmt.Sprintf("%v", vVal.Raw()),
			Timestamp:   ts,
		})
	}

	// Read repairs
	repairWMEs := eng.WorkingMemory().FindByClass("read_repair")
	for _, rw := range repairWMEs {
		targetVal, _ := rw.Get("target_node")
		repVal, _ := rw.Get("repaired_val")

		var ts int64 = 0
		if repTSVal, ok := rw.Get("repaired_ts"); ok {
			if v, ok := repTSVal.Raw().(int64); ok {
				ts = v
			}
		}

		res.ReadRepairs = append(res.ReadRepairs, ReadRepairEvent{
			TargetNode:  fmt.Sprintf("%v", targetVal.Raw()),
			RepairedVal: fmt.Sprintf("%v", repVal.Raw()),
			RepairedTS:  ts,
		})
	}

	// If Write succeeded, mutate cluster replica state in memory
	if qType == "write" && res.Success {
		for _, dc := range ce.cluster.DCs {
			for _, n := range dc.Nodes {
				if n.IsReplica && n.Health == data.HealthUp {
					n.Value = writeVal
					n.Timestamp = writeTS
				}
			}
		}
	}

	// If Read Repair triggered, update repaired replicas
	if qType == "read" && res.Success {
		for _, rr := range res.ReadRepairs {
			if n := ce.findNode(rr.TargetNode); n != nil {
				n.Value = rr.RepairedVal
				n.Timestamp = rr.RepairedTS
			}
		}
	}

	return res, nil
}

func (ce *CassandraEngine) findNode(id string) *data.Node {
	for _, dc := range ce.cluster.DCs {
		for _, n := range dc.Nodes {
			if n.ID == id {
				return n
			}
		}
	}
	return nil
}
