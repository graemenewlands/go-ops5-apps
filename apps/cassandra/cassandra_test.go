package cassandra

import (
	"strings"
	"testing"

	"github.com/graemenewlands/go-ops5-apps/apps/cassandra/data"
)

func TestAllNodesNormalConsistency(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	levels := []data.ConsistencyLevel{
		data.ConsistencyOne,
		data.ConsistencyTwo,
		data.ConsistencyThree,
		data.ConsistencyLocalQuorum,
		data.ConsistencyQuorum,
	}

	for _, cl := range levels {
		res, err := eng.ExecuteQuery("read", cl, "users_dataset", "")
		if err != nil {
			t.Fatalf("query error at %s: %v", cl, err)
		}
		if !res.Success {
			t.Fatalf("expected success at %s, got error: %s", cl, res.ErrorReason)
		}
		if res.AckResult.TotalAcks != 6 {
			t.Fatalf("expected 6 total acks when all nodes are UN, got %d", res.AckResult.TotalAcks)
		}
		if res.AckResult.LocalAcks != 3 {
			t.Fatalf("expected 3 local acks, got %d", res.AckResult.LocalAcks)
		}
		if res.Stats.CycleCount <= 0 {
			t.Fatalf("expected cycle count > 0, got %d", res.Stats.CycleCount)
		}
	}
}

func TestPullPlugAndLocalQuorum(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Coordinator is dc1-n1.
	// Pull plug on dc1-n2 (one local replica).
	// Replicas in DC1: dc1-n1 (UN), dc1-n2 (DS), dc1-n3 (UN).
	// Alive local replicas: 2. LOCAL_QUORUM requires 2.
	err = eng.PullPlug("dc1-n2")
	if err != nil {
		t.Fatalf("pull plug error: %v", err)
	}

	res, err := eng.ExecuteQuery("read", data.ConsistencyLocalQuorum, "users_dataset", "")
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected LOCAL_QUORUM to succeed with 2 of 3 local replicas alive, got: %s", res.ErrorReason)
	}
	if res.AckResult.LocalAcks != 2 {
		t.Fatalf("expected 2 local acks, got %d", res.AckResult.LocalAcks)
	}

	// Pull plug on second local replica: dc1-n3 (DS).
	// Now only dc1-n1 is alive locally (1 of 3).
	err = eng.PullPlug("dc1-n3")
	if err != nil {
		t.Fatalf("pull plug error: %v", err)
	}

	res2, err := eng.ExecuteQuery("read", data.ConsistencyLocalQuorum, "users_dataset", "")
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if res2.Success {
		t.Fatalf("expected LOCAL_QUORUM to fail with only 1 local replica alive")
	}
	if !strings.Contains(res2.ErrorReason, "LOCAL_QUORUM") {
		t.Fatalf("expected error mentioning LOCAL_QUORUM, got: %s", res2.ErrorReason)
	}

	// Global QUORUM needs 4 across cluster.
	// We have 1 local + 3 remote = 4 total alive!
	res3, err := eng.ExecuteQuery("read", data.ConsistencyQuorum, "users_dataset", "")
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if !res3.Success {
		t.Fatalf("expected QUORUM to succeed with 4 alive total replicas, got: %s", res3.ErrorReason)
	}
	if res3.AckResult.TotalAcks != 4 {
		t.Fatalf("expected 4 total acks for QUORUM, got %d", res3.AckResult.TotalAcks)
	}
}

func TestWriteHintedHandoff(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Pull plug on dc2-n1 (a remote replica)
	_ = eng.PullPlug("dc2-n1")

	// Execute write query with LOCAL_QUORUM
	res, err := eng.ExecuteQuery("write", data.ConsistencyLocalQuorum, "users_dataset", "new_user_val")
	if err != nil {
		t.Fatalf("write query error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected write to succeed at LOCAL_QUORUM, got: %s", res.ErrorReason)
	}

	// Should have generated a hinted handoff for the downed dc2-n1 node
	foundHint := false
	for _, h := range res.HintedHandoffs {
		if h.TargetNode == "dc2-n1" {
			foundHint = true
			if h.Value != "new_user_val" {
				t.Fatalf("expected hint value 'new_user_val', got %s", h.Value)
			}
			break
		}
	}
	if !foundHint {
		t.Fatalf("expected hinted handoff for down node dc2-n1, got: %v", res.HintedHandoffs)
	}
}

func TestReadRepairDetection(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Make dc1-n2 have an outdated timestamp (500 instead of 1000)
	node := eng.findNode("dc1-n2")
	node.Timestamp = 500
	node.Value = "outdated_v0"

	res, err := eng.ExecuteQuery("read", data.ConsistencyLocalQuorum, "users_dataset", "")
	if err != nil {
		t.Fatalf("read query error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected read success, got: %s", res.ErrorReason)
	}

	// Value should be resolved to the newer version (1000)
	if res.HighestTimestamp != 1000 {
		t.Fatalf("expected highest timestamp 1000, got %d", res.HighestTimestamp)
	}

	// Should detect read repair for dc1-n2
	foundRepair := false
	for _, rr := range res.ReadRepairs {
		if rr.TargetNode == "dc1-n2" {
			foundRepair = true
			if rr.RepairedTS != 1000 {
				t.Fatalf("expected repaired timestamp 1000, got %d", rr.RepairedTS)
			}
			break
		}
	}
	if !foundRepair {
		t.Fatalf("expected read repair detected for dc1-n2, got: %v", res.ReadRepairs)
	}
}

func TestInvalidStateEnumDJ(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	err = eng.SetNodeState("dc1-n1", data.HealthDown, data.MembershipJoining)
	if err == nil {
		t.Fatalf("expected error setting DJ state enum, but it was accepted")
	}
}

func TestCoordinatorDown(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Pull plug on coordinator dc1-n1
	_ = eng.PullPlug("dc1-n1")

	res, err := eng.ExecuteQuery("read", data.ConsistencyOne, "users_dataset", "")
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected failure when coordinator is down")
	}
	if !strings.Contains(res.ErrorReason, "NoHostAvailableException") {
		t.Fatalf("expected NoHostAvailableException, got: %s", res.ErrorReason)
	}
}

func TestCycleNodeState(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Start at UN
	node := eng.findNode("dc1-n1")
	if node.StateCode() != "UN" {
		t.Fatalf("expected UN, got %s", node.StateCode())
	}

	// UN -> DS
	next, _ := eng.CycleNodeState("dc1-n1")
	if next != "DS" {
		t.Fatalf("expected DS after UN, got %s", next)
	}

	// DS -> UJ
	next, _ = eng.CycleNodeState("dc1-n1")
	if next != "UJ" {
		t.Fatalf("expected UJ after DS, got %s", next)
	}

	// UJ -> UN
	next, _ = eng.CycleNodeState("dc1-n1")
	if next != "UN" {
		t.Fatalf("expected UN after UJ, got %s", next)
	}
}

func TestWANPartitionConsistency(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// 1. When WAN is connected, QUORUM should succeed
	res, err := eng.ExecuteQuery("read", data.ConsistencyQuorum, "users_dataset", "")
	if err != nil || !res.Success {
		t.Fatalf("expected QUORUM to succeed when WAN is connected, got: %v (%s)", err, res.ErrorReason)
	}

	// 2. Sever WAN link (Network partition between DC1 and DC2)
	eng.SetWANConnected(false)
	if eng.WANConnected() {
		t.Fatalf("expected WAN to be severed")
	}

	// QUORUM must fail when WAN is partitioned (requires 4 replicas across cluster, max 3 in local DC)
	resQuorum, err := eng.ExecuteQuery("read", data.ConsistencyQuorum, "users_dataset", "")
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if resQuorum.Success {
		t.Fatalf("expected QUORUM to fail during WAN network partition")
	}

	// LOCAL_QUORUM must SUCCEED during WAN partition (local DC has 3 replicas, only 2 needed)
	resLocal, err := eng.ExecuteQuery("write", data.ConsistencyLocalQuorum, "users_dataset", "partition_write")
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if !resLocal.Success {
		t.Fatalf("expected LOCAL_QUORUM to succeed during WAN partition, got: %s", resLocal.ErrorReason)
	}

	// Coordinator should have recorded hints for the 3 remote replicas across the severed WAN
	if len(resLocal.HintedHandoffs) != 3 {
		t.Fatalf("expected 3 hinted handoffs for remote replicas during WAN partition, got %d: %v", len(resLocal.HintedHandoffs), resLocal.HintedHandoffs)
	}

	// 3. Restore WAN link
	eng.ToggleWAN()
	if !eng.WANConnected() {
		t.Fatalf("expected WAN to be reconnected")
	}

	// QUORUM should succeed again
	resRestored, err := eng.ExecuteQuery("read", data.ConsistencyQuorum, "users_dataset", "")
	if err != nil || !resRestored.Success {
		t.Fatalf("expected QUORUM to succeed after WAN restored, got: %v (%s)", err, resRestored.ErrorReason)
	}
}
