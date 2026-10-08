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

func TestZeroDatacentersFailsAllConsistencyLevels(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	err = eng.SetDCCount(0)
	if err != nil {
		t.Fatalf("failed to set DC count to 0: %v", err)
	}

	if len(eng.ClusterConfig().DCs) != 0 {
		t.Fatalf("expected 0 DCs, got %d", len(eng.ClusterConfig().DCs))
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
			t.Fatalf("ExecuteQuery returned error at %s: %v", cl, err)
		}
		if res.Success {
			t.Fatalf("expected consistency level %s to FAIL with 0 datacenters, but it succeeded", cl)
		}
		if !strings.Contains(res.ErrorReason, "NoHostAvailableException") {
			t.Fatalf("expected NoHostAvailableException at %s with 0 DCs, got: %s", cl, res.ErrorReason)
		}
	}
}

func TestZeroReplicasFailsAllConsistencyLevels(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	err = eng.SetReplicasPerDC(0)
	if err != nil {
		t.Fatalf("failed to set replicas per DC to 0: %v", err)
	}

	if eng.ClusterConfig().ReplicasPerDC != 0 {
		t.Fatalf("expected 0 replicas per DC, got %d", eng.ClusterConfig().ReplicasPerDC)
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
			t.Fatalf("ExecuteQuery returned error at %s: %v", cl, err)
		}
		if res.Success {
			t.Fatalf("expected consistency level %s to FAIL with 0 replicas, but it succeeded", cl)
		}
		if !strings.Contains(res.ErrorReason, "UnavailableException") {
			t.Fatalf("expected UnavailableException at %s with 0 replicas, got: %s", cl, res.ErrorReason)
		}
	}
}

func TestAddRemoveDatacenters(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Initial: 2 DCs
	if len(eng.ClusterConfig().DCs) != 2 {
		t.Fatalf("expected 2 DCs initially, got %d", len(eng.ClusterConfig().DCs))
	}

	// Remove to 1 DC
	err = eng.RemoveDatacenter()
	if err != nil || len(eng.ClusterConfig().DCs) != 1 {
		t.Fatalf("expected 1 DC after removal, got %d: %v", len(eng.ClusterConfig().DCs), err)
	}

	// Remove to 0 DCs
	err = eng.RemoveDatacenter()
	if err != nil || len(eng.ClusterConfig().DCs) != 0 {
		t.Fatalf("expected 0 DCs after removal, got %d: %v", len(eng.ClusterConfig().DCs), err)
	}

	// Removing below 0 must error
	err = eng.RemoveDatacenter()
	if err == nil {
		t.Fatalf("expected error when removing below 0 DCs")
	}

	// Add up to 5 DCs
	for expected := 1; expected <= 5; expected++ {
		_, err = eng.AddDatacenter()
		if err != nil || len(eng.ClusterConfig().DCs) != expected {
			t.Fatalf("expected %d DCs after add, got %d: %v", expected, len(eng.ClusterConfig().DCs), err)
		}
	}

	// Adding above 5 must error
	_, err = eng.AddDatacenter()
	if err == nil {
		t.Fatalf("expected error when adding above 5 DCs in cluster")
	}
}

func TestFiveDatacentersQuorumAndPartition(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Set to 5 DCs with 3 replicas each (total 15 replicas)
	_ = eng.SetDCCount(5)
	_ = eng.SetReplicasPerDC(3)

	if len(eng.ClusterConfig().DCs) != 5 {
		t.Fatalf("expected 5 DCs, got %d", len(eng.ClusterConfig().DCs))
	}

	// QUORUM across 15 replicas requires floor(15/2) + 1 = 8 total acks.
	// All 15 replicas are UN -> QUORUM should succeed with 15 acks.
	resQ, err := eng.ExecuteQuery("read", data.ConsistencyQuorum, "users_dataset", "")
	if err != nil || !resQ.Success {
		t.Fatalf("expected QUORUM to succeed with 5 DCs (15 replicas), got: %v (%s)", err, resQ.ErrorReason)
	}
	if resQ.AckResult.TotalAcks != 15 {
		t.Fatalf("expected 15 total acks, got %d", resQ.AckResult.TotalAcks)
	}

	// LOCAL_QUORUM requires floor(3/2) + 1 = 2 local acks in coordinator DC.
	resLQ, err := eng.ExecuteQuery("read", data.ConsistencyLocalQuorum, "users_dataset", "")
	if err != nil || !resLQ.Success {
		t.Fatalf("expected LOCAL_QUORUM to succeed with 5 DCs, got: %v (%s)", err, resLQ.ErrorReason)
	}
	if resLQ.AckResult.LocalAcks != 3 {
		t.Fatalf("expected 3 local acks, got %d", resLQ.AckResult.LocalAcks)
	}

	// Now sever the WAN partition:
	eng.SetWANConnected(false)

	// LOCAL_QUORUM should STILL SUCCEED because all 3 local replicas in the coordinator's DC are reachable!
	resLQPart, err := eng.ExecuteQuery("read", data.ConsistencyLocalQuorum, "users_dataset", "")
	if err != nil || !resLQPart.Success {
		t.Fatalf("expected LOCAL_QUORUM to succeed during WAN partition, got: %v (%s)", err, resLQPart.ErrorReason)
	}

	// Global QUORUM requires 8 acks. But during WAN partition, only the 3 local replicas can reply!
	// 3 < 8, so QUORUM MUST FAIL with UnavailableException!
	resQPart, err := eng.ExecuteQuery("read", data.ConsistencyQuorum, "users_dataset", "")
	if err != nil {
		t.Fatalf("unexpected error executing query: %v", err)
	}
	if resQPart.Success {
		t.Fatalf("expected QUORUM to fail during WAN partition with 5 DCs (only 3 local acks out of 8 required)")
	}
}

func TestAddRemoveReplicas(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Start at 3
	if eng.ClusterConfig().ReplicasPerDC != 3 {
		t.Fatalf("expected 3 replicas initially, got %d", eng.ClusterConfig().ReplicasPerDC)
	}

	// Step down to 0
	for expected := 2; expected >= 0; expected-- {
		reps, err := eng.RemoveReplicaPerDC()
		if err != nil || reps != expected {
			t.Fatalf("expected %d replicas, got %d: %v", expected, reps, err)
		}
	}

	// Step below 0 must error
	_, err = eng.RemoveReplicaPerDC()
	if err == nil {
		t.Fatalf("expected error when removing below 0 replicas")
	}

	// Step up to 6
	for expected := 1; expected <= 6; expected++ {
		reps, err := eng.AddReplicaPerDC()
		if err != nil || reps != expected {
			t.Fatalf("expected %d replicas, got %d: %v", expected, reps, err)
		}
	}

	// Step above 6 must error
	_, err = eng.AddReplicaPerDC()
	if err == nil {
		t.Fatalf("expected error when adding above 6 replicas")
	}
}

func TestDynamicQuorumCalculation(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Set to 1 DC with 3 replicas
	_ = eng.SetDCCount(1)
	_ = eng.SetReplicasPerDC(3)

	// LOCAL_QUORUM: requires 2 local acks (floor(3/2) + 1). All 3 are UN -> should succeed with 3 acks.
	resLQ, err := eng.ExecuteQuery("read", data.ConsistencyLocalQuorum, "users_dataset", "")
	if err != nil || !resLQ.Success {
		t.Fatalf("expected LOCAL_QUORUM to succeed with 1 DC and 3 replicas, got: %v (%s)", err, resLQ.ErrorReason)
	}

	// QUORUM: with 1 DC and 3 total replicas, cluster quorum is floor(3/2) + 1 = 2. Should succeed.
	resQ, err := eng.ExecuteQuery("read", data.ConsistencyQuorum, "users_dataset", "")
	if err != nil || !resQ.Success {
		t.Fatalf("expected QUORUM to succeed with 1 DC and 3 replicas, got: %v (%s)", err, resQ.ErrorReason)
	}

	// Change to 2 DCs with 1 replica each (total 2 replicas, 1 local)
	_ = eng.SetDCCount(2)
	_ = eng.SetReplicasPerDC(1)

	// LOCAL_QUORUM: requires floor(1/2) + 1 = 1 local ack. Local DC has 1 UN replica -> should succeed!
	resLQ1, err := eng.ExecuteQuery("read", data.ConsistencyLocalQuorum, "users_dataset", "")
	if err != nil || !resLQ1.Success {
		t.Fatalf("expected LOCAL_QUORUM to succeed with 1 local replica, got: %v (%s)", err, resLQ1.ErrorReason)
	}

	// QUORUM: requires floor(2/2) + 1 = 2 total acks. Both DCs have 1 UN replica (2 total) -> should succeed!
	resQ1, err := eng.ExecuteQuery("read", data.ConsistencyQuorum, "users_dataset", "")
	if err != nil || !resQ1.Success {
		t.Fatalf("expected QUORUM to succeed with 2 total replicas across 2 DCs, got: %v (%s)", err, resQ1.ErrorReason)
	}
}

func TestPartitionBetweenDC3AndDC4(t *testing.T) {
	eng, err := NewCassandraEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Scale to 5 DCs (3 replicas each -> 15 total replicas)
	if err := eng.SetDCCount(5); err != nil {
		t.Fatalf("failed to scale to 5 DCs: %v", err)
	}

	links := eng.GetWANLinks()
	if len(links) != 4 {
		t.Fatalf("expected 4 WAN links for 5 DCs, got %d", len(links))
	}

	// Sever link between DC3 and DC4
	connected := eng.ToggleWANLink("dc3", "dc4")
	if connected {
		t.Fatalf("expected link dc3-dc4 to be severed (connected=false)")
	}

	// Verify reachability in Partition 1 (DC1, DC2, DC3)
	if !eng.IsReachable("dc1", "dc2") || !eng.IsReachable("dc1", "dc3") || !eng.IsReachable("dc2", "dc3") {
		t.Fatalf("expected DC1, DC2, DC3 to be mutually reachable")
	}

	// Verify reachability in Partition 2 (DC4, DC5)
	if !eng.IsReachable("dc4", "dc5") {
		t.Fatalf("expected DC4 and DC5 to be mutually reachable")
	}

	// Verify cross-partition disconnection
	if eng.IsReachable("dc1", "dc4") || eng.IsReachable("dc1", "dc5") || eng.IsReachable("dc2", "dc4") || eng.IsReachable("dc3", "dc4") {
		t.Fatalf("expected DC1..DC3 and DC4..DC5 to be partitioned")
	}

	// Coordinator in DC1 (Partition 1):
	// Total replicas in cluster = 15. Required quorum = floor(15/2) + 1 = 8.
	// Partition 1 has DC1 (3), DC2 (3), DC3 (3) = 9 alive replicas.
	// 9 >= 8, so QUORUM from DC1 MUST SUCCEED!
	resP1, err := eng.ExecuteQuery("read", data.ConsistencyQuorum, "users_dataset", "", "dc1-n1")
	if err != nil || !resP1.Success {
		t.Fatalf("expected QUORUM in partition 1 (9 replicas >= 8 required) to SUCCEED, got: %v (%s)", err, resP1.ErrorReason)
	}
	if resP1.AckResult.TotalAcks != 9 {
		t.Fatalf("expected 9 acks from partition 1, got %d", resP1.AckResult.TotalAcks)
	}

	// Coordinator in DC4 (Partition 2):
	// Partition 2 has DC4 (3) and DC5 (3) = 6 alive replicas.
	// 6 < 8 required quorum, so QUORUM from DC4 MUST FAIL with UnavailableException!
	resP2, err := eng.ExecuteQuery("read", data.ConsistencyQuorum, "users_dataset", "", "dc4-n1")
	if err != nil {
		t.Fatalf("unexpected engine error: %v", err)
	}
	if resP2.Success {
		t.Fatalf("expected QUORUM in partition 2 (6 replicas < 8 required) to FAIL, but succeeded")
	}

	// But LOCAL_QUORUM in DC4 only requires floor(3/2) + 1 = 2 local acks in DC4 -> MUST SUCCEED!
	resP2LQ, err := eng.ExecuteQuery("read", data.ConsistencyLocalQuorum, "users_dataset", "", "dc4-n1")
	if err != nil || !resP2LQ.Success {
		t.Fatalf("expected LOCAL_QUORUM in partition 2 to SUCCEED, got: %v (%s)", err, resP2LQ.ErrorReason)
	}
	if resP2LQ.AckResult.LocalAcks != 3 {
		t.Fatalf("expected 3 local acks in DC4, got %d", resP2LQ.AckResult.LocalAcks)
	}

	// Now reconnect link dc3-dc4
	nowConnected := eng.ToggleWANLink("dc3", "dc4")
	if !nowConnected {
		t.Fatalf("expected link dc3-dc4 to be reconnected (connected=true)")
	}

	// Now QUORUM from DC4 can reach all 15 replicas and MUST SUCCEED!
	resP2Restored, err := eng.ExecuteQuery("read", data.ConsistencyQuorum, "users_dataset", "", "dc4-n1")
	if err != nil || !resP2Restored.Success {
		t.Fatalf("expected QUORUM from DC4 to SUCCEED after restoring WAN link, got: %v (%s)", err, resP2Restored.ErrorReason)
	}
	if resP2Restored.AckResult.TotalAcks != 15 {
		t.Fatalf("expected 15 acks after restoring WAN, got %d", resP2Restored.AckResult.TotalAcks)
	}
}
