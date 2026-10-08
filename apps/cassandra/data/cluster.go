package data

import "fmt"

// NodeHealth represents network/process health (U=Up, D=Down).
type NodeHealth string

const (
	HealthUp   NodeHealth = "U"
	HealthDown NodeHealth = "D"
)

// NodeMembership represents cluster lifecycle/membership (S=Stopped, J=Joining, N=Normal).
type NodeMembership string

const (
	MembershipStopped NodeMembership = "S"
	MembershipJoining NodeMembership = "J"
	MembershipNormal  NodeMembership = "N"
)

// NodeState is the combined Cassandra node state (e.g. UN, UJ, DS, DN).
// Note: DJ is explicitly not a valid state enum.
type NodeState struct {
	Health     NodeHealth     `json:"health"`
	Membership NodeMembership `json:"membership"`
}

// String returns the 2-letter state code, e.g. "UN", "UJ", "DS", "DN".
func (ns NodeState) String() string {
	return string(ns.Health) + string(ns.Membership)
}

// Node represents a single node in a Cassandra ring.
type Node struct {
	ID         string         `json:"id"`
	DC         string         `json:"dc"`
	RingPos    int            `json:"ringPos"` // 1 to 6 (clockwise order)
	Health     NodeHealth     `json:"health"`
	Membership NodeMembership `json:"membership"`
	IsReplica  bool           `json:"isReplica"`  // True if node holds replica for target dataset
	Value      string         `json:"value"`      // Stored data value
	Timestamp  int64          `json:"timestamp"`  // Lamport / write timestamp
	TokenStart int64          `json:"tokenStart"` // Token range start
	TokenEnd   int64          `json:"tokenEnd"`   // Token range end
}

// StateCode returns "UN", "UJ", "DS", "DN".
func (n *Node) StateCode() string {
	return string(n.Health) + string(n.Membership)
}

// IsOperational returns true if the node is Up and Normal.
func (n *Node) IsOperational() bool {
	return n.Health == HealthUp && n.Membership == MembershipNormal
}

// Datacenter represents a Cassandra datacenter ring containing 6 nodes.
type Datacenter struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Nodes []*Node `json:"nodes"`
}

// ConsistencyLevel defines Cassandra query consistency levels.
type ConsistencyLevel string

const (
	ConsistencyOne         ConsistencyLevel = "ONE"
	ConsistencyTwo         ConsistencyLevel = "TWO"
	ConsistencyThree       ConsistencyLevel = "THREE"
	ConsistencyLocalQuorum ConsistencyLevel = "LOCAL_QUORUM"
	ConsistencyQuorum      ConsistencyLevel = "QUORUM"
)

// ClusterConfig defines default cluster initialization.
type ClusterConfig struct {
	NodesPerDC     int              `json:"nodesPerDc"`
	ReplicasPerDC  int              `json:"replicasPerDc"`
	DefaultDataset string           `json:"defaultDataset"`
	DefaultValue   string           `json:"defaultValue"`
	DCs            []*Datacenter    `json:"dcs"`
	Consistency    ConsistencyLevel `json:"consistency"`
}

// CreateDatacenter initializes a new datacenter with nodeCount nodes and repsPerDC replicas.
func CreateDatacenter(id, name string, nodeCount, repsPerDC int) *Datacenter {
	nodes := make([]*Node, nodeCount)
	for i := 1; i <= nodeCount; i++ {
		isReplica := (i <= repsPerDC)
		val := ""
		var ts int64 = 0
		if isReplica {
			val = "payload_v1"
			ts = 1000
		}
		nodes[i-1] = &Node{
			ID:         fmt.Sprintf("%s-n%d", id, i),
			DC:         id,
			RingPos:    i,
			Health:     HealthUp,
			Membership: MembershipNormal,
			IsReplica:  isReplica,
			Value:      val,
			Timestamp:  ts,
			TokenStart: int64((i - 1) * 60),
			TokenEnd:   int64(i * 60),
		}
	}
	return &Datacenter{
		ID:    id,
		Name:  name,
		Nodes: nodes,
	}
}

// DefaultCluster returns a 2-datacenter cluster, each with 6 nodes, where 3 nodes per DC are replicas.
func DefaultCluster() *ClusterConfig {
	return &ClusterConfig{
		NodesPerDC:     6,
		ReplicasPerDC:  3,
		DefaultDataset: "users_dataset",
		DefaultValue:   "payload_v1",
		DCs: []*Datacenter{
			CreateDatacenter("dc1", "Datacenter 1 (East)", 6, 3),
			CreateDatacenter("dc2", "Datacenter 2 (West)", 6, 3),
		},
		Consistency: ConsistencyLocalQuorum,
	}
}
