//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"syscall/js"

	"github.com/graemenewlands/go-ops5-apps/apps/cassandra"
	"github.com/graemenewlands/go-ops5-apps/apps/cassandra/data"
)

var currentEngine *cassandra.CassandraEngine

func safeCallback(fn func(this js.Value, args []js.Value) any) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) (result any) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC recovered in wasm callback: %v\n%s", r, debug.Stack())
				result = map[string]any{
					"success": false,
					"error":   fmt.Sprintf("WASM panic: %v", r),
				}
			}
		}()
		return fn(this, args)
	})
}

func main() {
	c := make(chan struct{}, 0)

	// Pre-initialize Cassandra engine during startup
	eng, err := cassandra.NewCassandraEngine(nil)
	if err != nil {
		log.Printf("Failed to initialize Cassandra engine: %v", err)
	} else {
		currentEngine = eng
	}

	// Register JS callbacks with safe panic recovery
	js.Global().Set("cassandraInit", safeCallback(cassandraInit))
	js.Global().Set("cassandraGetCluster", safeCallback(cassandraGetCluster))
	js.Global().Set("cassandraReset", safeCallback(cassandraReset))
	js.Global().Set("cassandraSetClientDC", safeCallback(cassandraSetClientDC))
	js.Global().Set("cassandraSetCoordinator", safeCallback(cassandraSetCoordinator))
	js.Global().Set("cassandraPullPlug", safeCallback(cassandraPullPlug))
	js.Global().Set("cassandraCycleNodeState", safeCallback(cassandraCycleNodeState))
	js.Global().Set("cassandraSetNodeState", safeCallback(cassandraSetNodeState))
	js.Global().Set("cassandraExecuteQuery", safeCallback(cassandraExecuteQuery))
	js.Global().Set("cassandraToggleWAN", safeCallback(cassandraToggleWAN))
	js.Global().Set("cassandraSetWAN", safeCallback(cassandraSetWAN))
	js.Global().Set("cassandraToggleWANLink", safeCallback(cassandraToggleWANLink))
	js.Global().Set("cassandraSetWANLink", safeCallback(cassandraSetWANLink))
	js.Global().Set("cassandraAddDC", safeCallback(cassandraAddDC))
	js.Global().Set("cassandraRemoveDC", safeCallback(cassandraRemoveDC))
	js.Global().Set("cassandraAddReplica", safeCallback(cassandraAddReplica))
	js.Global().Set("cassandraRemoveReplica", safeCallback(cassandraRemoveReplica))
	js.Global().Set("cassandraSetDCCount", safeCallback(cassandraSetDCCount))
	js.Global().Set("cassandraSetReplicasPerDC", safeCallback(cassandraSetReplicasPerDC))
	js.Global().Set("cassandraGetRuleSource", safeCallback(cassandraGetRuleSource))

	// Signal to JS that Wasm module is ready asynchronously via setTimeout to avoid re-entrancy
	if readyFn := js.Global().Get("onOps5CassandraReady"); readyFn.Type() == js.TypeFunction {
		js.Global().Call("setTimeout", readyFn, 0)
	}

	<-c
}

func cassandraInit(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		eng, err := cassandra.NewCassandraEngine(nil)
		if err != nil {
			return map[string]any{"success": false, "error": err.Error()}
		}
		currentEngine = eng
	} else {
		currentEngine.ResetCluster()
	}

	return getClusterState()
}

func cassandraGetCluster(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	return getClusterState()
}

func cassandraReset(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	currentEngine.ResetCluster()
	return getClusterState()
}

func cassandraSetClientDC(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return map[string]any{"success": false, "error": "missing DC ID"}
	}

	dcID := args[0].String()
	if err := currentEngine.SetClientDC(dcID); err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	return getClusterState()
}

func cassandraSetCoordinator(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return map[string]any{"success": false, "error": "missing node ID"}
	}

	nodeID := args[0].String()
	if err := currentEngine.SetCoordinator(nodeID); err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	return getClusterState()
}

func cassandraPullPlug(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return map[string]any{"success": false, "error": "missing node ID"}
	}

	nodeID := args[0].String()
	if err := currentEngine.PullPlug(nodeID); err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	return getClusterState()
}

func cassandraCycleNodeState(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return map[string]any{"success": false, "error": "missing node ID"}
	}

	nodeID := args[0].String()
	nextState, err := currentEngine.CycleNodeState(nodeID)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}

	state := getClusterState()
	state["cycledState"] = nextState
	return state
}

func cassandraSetNodeState(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	if len(args) < 3 {
		return map[string]any{"success": false, "error": "expected nodeID, health, membership"}
	}

	nodeID := args[0].String()
	health := data.NodeHealth(args[1].String())
	membership := data.NodeMembership(args[2].String())

	if err := currentEngine.SetNodeState(nodeID, health, membership); err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	return getClusterState()
}

func cassandraExecuteQuery(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}

	qType := "read"
	cl := data.ConsistencyLocalQuorum
	key := "users_dataset"
	writeVal := ""
	coordID := ""

	if len(args) >= 1 && args[0].Type() == js.TypeString && args[0].String() != "" {
		qType = args[0].String()
	}
	if len(args) >= 2 && args[1].Type() == js.TypeString && args[1].String() != "" {
		cl = data.ConsistencyLevel(args[1].String())
	}
	if len(args) >= 3 && args[2].Type() == js.TypeString && args[2].String() != "" {
		key = args[2].String()
	}
	if len(args) >= 4 && args[3].Type() == js.TypeString {
		writeVal = args[3].String()
	}
	if len(args) >= 5 && args[4].Type() == js.TypeString && args[4].String() != "" {
		coordID = args[4].String()
	}

	res, err := currentEngine.ExecuteQuery(qType, cl, key, writeVal, coordID)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}

	bytes, err := json.Marshal(res)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}

	var parsed map[string]any
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}

	clusterState := getClusterState()
	parsed["cluster"] = clusterState["cluster"]
	parsed["wanConnected"] = clusterState["wanConnected"]
	parsed["wanLinks"] = clusterState["wanLinks"]
	return parsed
}

func cassandraToggleWAN(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	currentEngine.ToggleWAN()
	return getClusterState()
}

func cassandraSetWAN(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	if len(args) < 1 {
		return map[string]any{"success": false, "error": "missing boolean argument"}
	}
	connected := args[0].Bool()
	currentEngine.SetWANConnected(connected)
	return getClusterState()
}

func cassandraToggleWANLink(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	if len(args) < 2 {
		return map[string]any{"success": false, "error": "missing dc arguments"}
	}
	dc1 := args[0].String()
	dc2 := args[1].String()
	currentEngine.ToggleWANLink(dc1, dc2)
	return getClusterState()
}

func cassandraSetWANLink(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	if len(args) < 3 {
		return map[string]any{"success": false, "error": "missing arguments"}
	}
	dc1 := args[0].String()
	dc2 := args[1].String()
	connected := args[2].Bool()
	currentEngine.SetWANLink(dc1, dc2, connected)
	return getClusterState()
}

func cassandraAddDC(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	_, err := currentEngine.AddDatacenter()
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	return getClusterState()
}

func cassandraRemoveDC(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	err := currentEngine.RemoveDatacenter()
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	return getClusterState()
}

func cassandraAddReplica(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	_, err := currentEngine.AddReplicaPerDC()
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	return getClusterState()
}

func cassandraRemoveReplica(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	_, err := currentEngine.RemoveReplicaPerDC()
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	return getClusterState()
}

func cassandraSetDCCount(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	if len(args) < 1 {
		return map[string]any{"success": false, "error": "missing count argument"}
	}
	count := args[0].Int()
	err := currentEngine.SetDCCount(count)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	return getClusterState()
}

func cassandraSetReplicasPerDC(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	if len(args) < 1 {
		return map[string]any{"success": false, "error": "missing replicas argument"}
	}
	reps := args[0].Int()
	err := currentEngine.SetReplicasPerDC(reps)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	return getClusterState()
}

func cassandraGetRuleSource(this js.Value, args []js.Value) any {
	return cassandra.CassandraRulesSource
}

func getClusterState() map[string]any {
	cfg := currentEngine.ClusterConfig()
	bytes, _ := json.Marshal(cfg)
	var parsedCfg any
	_ = json.Unmarshal(bytes, &parsedCfg)

	dcCount := len(cfg.DCs)
	wanLinks := currentEngine.GetWANLinks()
	var jsLinks []any
	for _, l := range wanLinks {
		jsLinks = append(jsLinks, map[string]any{
			"dc1":       l.DC1,
			"dc2":       l.DC2,
			"connected": l.Connected,
		})
	}

	return map[string]any{
		"success":       true,
		"cluster":       parsedCfg,
		"clientDc":      currentEngine.ClientDC(),
		"coordinatorId": currentEngine.CoordinatorID(),
		"wanConnected":  currentEngine.WANConnected(),
		"wanLinks":      jsLinks,
		"dcCount":       dcCount,
		"replicasPerDc": cfg.ReplicasPerDC,
	}
}
