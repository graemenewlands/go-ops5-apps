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

// WANLinkState represents the state of a WAN link between two adjacent datacenters.
type WANLinkState struct {
	DC1       string `json:"dc1"`
	DC2       string `json:"dc2"`
	Connected bool   `json:"connected"`
}

func linkKey(dc1, dc2 string) string {
	if dc1 > dc2 {
		dc1, dc2 = dc2, dc1
	}
	return dc1 + ":" + dc2
}

// CassandraEngine wraps the OPS5 rule engine and cluster state.
type CassandraEngine struct {
	mu            sync.RWMutex
	cluster       *data.ClusterConfig
	clientDC      string
	coordinatorID string
	wanConnected  bool // True = all active WAN links operational, False = at least one partition
	severedLinks  map[string]bool // normalized "dcA:dcB" -> true if severed
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
		severedLinks:  make(map[string]bool),
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

// GetWANLinks returns the active adjacent WAN links across current datacenters.
func (ce *CassandraEngine) GetWANLinks() []WANLinkState {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	var links []WANLinkState
	for i := 0; i < len(ce.cluster.DCs)-1; i++ {
		dc1 := ce.cluster.DCs[i].ID
		dc2 := ce.cluster.DCs[i+1].ID
		key := linkKey(dc1, dc2)
		severed := ce.severedLinks != nil && ce.severedLinks[key]
		links = append(links, WANLinkState{
			DC1:       dc1,
			DC2:       dc2,
			Connected: !severed,
		})
	}
	return links
}

// SetWANLink sets the operational state of a specific WAN link between two datacenters.
func (ce *CassandraEngine) SetWANLink(dc1, dc2 string, connected bool) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if ce.severedLinks == nil {
		ce.severedLinks = make(map[string]bool)
	}
	key := linkKey(dc1, dc2)
	if connected {
		delete(ce.severedLinks, key)
	} else {
		ce.severedLinks[key] = true
	}
	ce.updateGlobalWANFlagLocked()
}

// ToggleWANLink toggles the state of a specific WAN link between operational and severed.
func (ce *CassandraEngine) ToggleWANLink(dc1, dc2 string) bool {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if ce.severedLinks == nil {
		ce.severedLinks = make(map[string]bool)
	}
	key := linkKey(dc1, dc2)
	nowSevered := !ce.severedLinks[key]
	if nowSevered {
		ce.severedLinks[key] = true
	} else {
		delete(ce.severedLinks, key)
	}
	ce.updateGlobalWANFlagLocked()
	return !nowSevered
}

func (ce *CassandraEngine) updateGlobalWANFlagLocked() {
	allConnected := true
	for i := 0; i < len(ce.cluster.DCs)-1; i++ {
		dc1 := ce.cluster.DCs[i].ID
		dc2 := ce.cluster.DCs[i+1].ID
		if ce.severedLinks != nil && ce.severedLinks[linkKey(dc1, dc2)] {
			allConnected = false
			break
		}
	}
	ce.wanConnected = allConnected
}

// IsReachable checks whether two datacenters can communicate across unpartitioned WAN links.
func (ce *CassandraEngine) IsReachable(dcA, dcB string) bool {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.isReachableLocked(dcA, dcB)
}

func (ce *CassandraEngine) isReachableLocked(dcA, dcB string) bool {
	if dcA == dcB {
		return true
	}
	idxA, idxB := -1, -1
	for i, dc := range ce.cluster.DCs {
		if dc.ID == dcA {
			idxA = i
		}
		if dc.ID == dcB {
			idxB = i
		}
	}
	if idxA == -1 || idxB == -1 {
		return false
	}
	start, end := idxA, idxB
	if start > end {
		start, end = end, start
	}
	for i := start; i < end; i++ {
		d1 := ce.cluster.DCs[i].ID
		d2 := ce.cluster.DCs[i+1].ID
		if ce.severedLinks != nil && ce.severedLinks[linkKey(d1, d2)] {
			return false
		}
	}
	return true
}

// WANConnected returns true if all inter-datacenter WAN links are operational.
func (ce *CassandraEngine) WANConnected() bool {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.wanConnected
}

// SetWANConnected sets all inter-datacenter WAN links to connected or severed.
func (ce *CassandraEngine) SetWANConnected(connected bool) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if connected {
		ce.severedLinks = make(map[string]bool)
		ce.wanConnected = true
	} else {
		if ce.severedLinks == nil {
			ce.severedLinks = make(map[string]bool)
		}
		for i := 0; i < len(ce.cluster.DCs)-1; i++ {
			key := linkKey(ce.cluster.DCs[i].ID, ce.cluster.DCs[i+1].ID)
			ce.severedLinks[key] = true
		}
		ce.wanConnected = false
	}
}

// ToggleWAN toggles all inter-datacenter WAN links between fully operational and severed.
func (ce *CassandraEngine) ToggleWAN() bool {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if ce.wanConnected {
		if ce.severedLinks == nil {
			ce.severedLinks = make(map[string]bool)
		}
		for i := 0; i < len(ce.cluster.DCs)-1; i++ {
			key := linkKey(ce.cluster.DCs[i].ID, ce.cluster.DCs[i+1].ID)
			ce.severedLinks[key] = true
		}
		ce.wanConnected = false
	} else {
		ce.severedLinks = make(map[string]bool)
		ce.wanConnected = true
	}
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

// DefaultDatacenterTemplates defines up to 5 datacenters available for dynamic cluster expansion.
var DefaultDatacenterTemplates = []struct {
	ID   string
	Name string
}{
	{"dc1", "Datacenter 1 (US East)"},
	{"dc2", "Datacenter 2 (US West)"},
	{"dc3", "Datacenter 3 (EU Central)"},
	{"dc4", "Datacenter 4 (AP South)"},
	{"dc5", "Datacenter 5 (SA East)"},
}

// AddDatacenter adds a datacenter to the cluster (up to 5 DCs).
func (ce *CassandraEngine) AddDatacenter() (*data.Datacenter, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if len(ce.cluster.DCs) >= len(DefaultDatacenterTemplates) {
		return nil, fmt.Errorf("maximum %d datacenters in cluster", len(DefaultDatacenterTemplates))
	}

	existingIDs := make(map[string]bool)
	for _, dc := range ce.cluster.DCs {
		existingIDs[dc.ID] = true
	}

	var template *struct{ ID, Name string }
	for i := range DefaultDatacenterTemplates {
		if !existingIDs[DefaultDatacenterTemplates[i].ID] {
			template = &DefaultDatacenterTemplates[i]
			break
		}
	}
	if template == nil {
		return nil, fmt.Errorf("no more datacenter templates available")
	}

	newDC := data.CreateDatacenter(template.ID, template.Name, 6, ce.cluster.ReplicasPerDC)
	ce.cluster.DCs = append(ce.cluster.DCs, newDC)

	if len(ce.cluster.DCs) == 1 {
		ce.clientDC = newDC.ID
		if len(newDC.Nodes) > 0 {
			ce.coordinatorID = newDC.Nodes[0].ID
		}
	}

	return newDC, nil
}

// RemoveDatacenter removes a datacenter from the cluster (down to 0 DCs).
func (ce *CassandraEngine) RemoveDatacenter() error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if len(ce.cluster.DCs) == 0 {
		return fmt.Errorf("cluster already has 0 datacenters")
	}

	removedDC := ce.cluster.DCs[len(ce.cluster.DCs)-1]
	ce.cluster.DCs = ce.cluster.DCs[:len(ce.cluster.DCs)-1]

	if len(ce.cluster.DCs) == 0 {
		ce.clientDC = ""
		ce.coordinatorID = ""
	} else if ce.clientDC == removedDC.ID {
		ce.clientDC = ce.cluster.DCs[0].ID
		if len(ce.cluster.DCs[0].Nodes) > 0 {
			ce.coordinatorID = ce.cluster.DCs[0].Nodes[0].ID
		} else {
			ce.coordinatorID = ""
		}
	}
	return nil
}

// AddReplicaPerDC increments replicas per DC (up to 6).
func (ce *CassandraEngine) AddReplicaPerDC() (int, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if ce.cluster.ReplicasPerDC >= 6 {
		return ce.cluster.ReplicasPerDC, fmt.Errorf("maximum 6 replicas per DC")
	}

	ce.cluster.ReplicasPerDC++
	ce.applyReplicasPerDC()
	return ce.cluster.ReplicasPerDC, nil
}

// RemoveReplicaPerDC decrements replicas per DC (down to 0).
func (ce *CassandraEngine) RemoveReplicaPerDC() (int, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if ce.cluster.ReplicasPerDC <= 0 {
		return ce.cluster.ReplicasPerDC, fmt.Errorf("minimum 0 replicas per DC")
	}

	ce.cluster.ReplicasPerDC--
	ce.applyReplicasPerDC()
	return ce.cluster.ReplicasPerDC, nil
}

// SetDCCount sets the datacenter count directly (0 to 5).
func (ce *CassandraEngine) SetDCCount(count int) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if count < 0 || count > len(DefaultDatacenterTemplates) {
		return fmt.Errorf("datacenter count must be between 0 and %d", len(DefaultDatacenterTemplates))
	}

	for len(ce.cluster.DCs) < count {
		existingIDs := make(map[string]bool)
		for _, dc := range ce.cluster.DCs {
			existingIDs[dc.ID] = true
		}
		for i := range DefaultDatacenterTemplates {
			if !existingIDs[DefaultDatacenterTemplates[i].ID] {
				newDC := data.CreateDatacenter(DefaultDatacenterTemplates[i].ID, DefaultDatacenterTemplates[i].Name, 6, ce.cluster.ReplicasPerDC)
				ce.cluster.DCs = append(ce.cluster.DCs, newDC)
				break
			}
		}
	}

	for len(ce.cluster.DCs) > count {
		ce.cluster.DCs = ce.cluster.DCs[:len(ce.cluster.DCs)-1]
	}

	if len(ce.cluster.DCs) == 0 {
		ce.clientDC = ""
		ce.coordinatorID = ""
	} else if ce.findNode(ce.coordinatorID) == nil {
		ce.clientDC = ce.cluster.DCs[0].ID
		ce.coordinatorID = ce.cluster.DCs[0].Nodes[0].ID
	}

	return nil
}

// SetReplicasPerDC sets the number of replicas per DC directly (0 to 6).
func (ce *CassandraEngine) SetReplicasPerDC(reps int) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if reps < 0 || reps > 6 {
		return fmt.Errorf("replicas per DC must be between 0 and 6")
	}

	ce.cluster.ReplicasPerDC = reps
	ce.applyReplicasPerDC()
	return nil
}

func (ce *CassandraEngine) applyReplicasPerDC() {
	for _, dc := range ce.cluster.DCs {
		for _, n := range dc.Nodes {
			n.IsReplica = (n.RingPos <= ce.cluster.ReplicasPerDC)
			if n.IsReplica && n.Value == "" {
				n.Value = "payload_v1"
				n.Timestamp = 1000
			}
		}
	}
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

	// If 0 datacenters exist, fail immediately with NoHostAvailableException
	if len(ce.cluster.DCs) == 0 {
		return &QueryResult{
			QueryID:          queryID,
			Success:          false,
			ErrorReason:      "NoHostAvailableException: 0 datacenters available in cluster",
			QueryType:        qType,
			TargetKey:        key,
			ConsistencyLevel: string(cl),
			Coordinator:      "none",
			LocalDC:          "none",
			Stats: EngineStats{
				CycleCount: 0,
				ElapsedMs:  0,
				WMECounter: 0,
			},
		}, nil
	}

	activeCoord := ce.coordinatorID
	activeDC := ce.clientDC
	if len(optCoord) > 0 && optCoord[0] != "" {
		if node := ce.findNode(optCoord[0]); node != nil {
			activeCoord = node.ID
			activeDC = node.DC
		}
	}
	if ce.findNode(activeCoord) == nil && len(ce.cluster.DCs) > 0 && len(ce.cluster.DCs[0].Nodes) > 0 {
		activeCoord = ce.cluster.DCs[0].Nodes[0].ID
		activeDC = ce.cluster.DCs[0].ID
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
	reqLQ := int64(999)
	if ce.cluster.ReplicasPerDC > 0 {
		reqLQ = int64((ce.cluster.ReplicasPerDC / 2) + 1)
	}
	totalReps := int64(ce.cluster.ReplicasPerDC * len(ce.cluster.DCs))
	reqQ := int64(999)
	if totalReps > 0 {
		reqQ = int64((totalReps / 2) + 1)
	}

	eng.Make("cluster_meta", map[string]model.Value{
		"total_replicas":   model.NewInt(totalReps),
		"local_replicas":   model.NewInt(int64(ce.cluster.ReplicasPerDC)),
		"req_local_quorum": model.NewInt(reqLQ),
		"req_quorum":       model.NewInt(reqQ),
		"wan_connected":    model.NewInt(wanVal),
	})

	// 1b. Assert inter-DC reachability WMEs based on WAN link state
	for _, dcA := range ce.cluster.DCs {
		for _, dcB := range ce.cluster.DCs {
			if dcA.ID != dcB.ID && ce.isReachableLocked(dcA.ID, dcB.ID) {
				eng.Make("dc_reachable", map[string]model.Value{
					"from_dc": model.NewSymbol(dcA.ID),
					"to_dc":   model.NewSymbol(dcB.ID),
				})
			}
		}
	}

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
			res.LiveTally.LocalAlive = int(toInt64(loc.Raw()))
		}
		if rem, ok := tw.Get("remote_alive"); ok {
			res.LiveTally.RemoteAlive = int(toInt64(rem.Raw()))
		}
		if tot, ok := tw.Get("total_alive"); ok {
			res.LiveTally.TotalAlive = int(toInt64(tot.Raw()))
		}
	}

	// Ack accumulator
	accWMEs := eng.WorkingMemory().FindByClass("ack_accumulator")
	if len(accWMEs) > 0 {
		aw := accWMEs[0]
		if loc, ok := aw.Get("local_acks"); ok {
			res.AckResult.LocalAcks = int(toInt64(loc.Raw()))
		}
		if rem, ok := aw.Get("remote_acks"); ok {
			res.AckResult.RemoteAcks = int(toInt64(rem.Raw()))
		}
		if tot, ok := aw.Get("total_acks"); ok {
			res.AckResult.TotalAcks = int(toInt64(tot.Raw()))
		}
		if hi, ok := aw.Get("highest_ts"); ok {
			res.HighestTimestamp = toInt64(hi.Raw())
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

func toInt64(val any) int64 {
	switch v := val.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	default:
		return 0
	}
}
