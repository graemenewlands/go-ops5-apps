//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/graemenewlands/go-ops5-apps/apps/cassandra"
	"github.com/graemenewlands/go-ops5-apps/apps/cassandra/data"
)

var currentEngine *cassandra.CassandraEngine

func main() {
	c := make(chan struct{}, 0)

	// Register JS callbacks
	js.Global().Set("cassandraInit", js.FuncOf(cassandraInit))
	js.Global().Set("cassandraGetCluster", js.FuncOf(cassandraGetCluster))
	js.Global().Set("cassandraReset", js.FuncOf(cassandraReset))
	js.Global().Set("cassandraSetClientDC", js.FuncOf(cassandraSetClientDC))
	js.Global().Set("cassandraSetCoordinator", js.FuncOf(cassandraSetCoordinator))
	js.Global().Set("cassandraPullPlug", js.FuncOf(cassandraPullPlug))
	js.Global().Set("cassandraCycleNodeState", js.FuncOf(cassandraCycleNodeState))
	js.Global().Set("cassandraSetNodeState", js.FuncOf(cassandraSetNodeState))
	js.Global().Set("cassandraExecuteQuery", js.FuncOf(cassandraExecuteQuery))
	js.Global().Set("cassandraToggleWAN", js.FuncOf(cassandraToggleWAN))
	js.Global().Set("cassandraSetWAN", js.FuncOf(cassandraSetWAN))
	js.Global().Set("cassandraGetRuleSource", js.FuncOf(cassandraGetRuleSource))

	// Signal to JS that Wasm module is ready
	if readyFn := js.Global().Get("onOps5CassandraReady"); readyFn.Type() == js.TypeFunction {
		readyFn.Invoke()
	}

	<-c
}

func cassandraInit(this js.Value, args []js.Value) any {
	eng, err := cassandra.NewCassandraEngine(nil)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	currentEngine = eng

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

	parsed["cluster"] = getClusterState()["cluster"]
	parsed["wanConnected"] = currentEngine.WANConnected()
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

func cassandraGetRuleSource(this js.Value, args []js.Value) any {
	return cassandra.CassandraRulesSource
}

func getClusterState() map[string]any {
	cfg := currentEngine.ClusterConfig()
	bytes, _ := json.Marshal(cfg)
	var parsedCfg any
	_ = json.Unmarshal(bytes, &parsedCfg)

	return map[string]any{
		"success":       true,
		"cluster":       parsedCfg,
		"clientDc":      currentEngine.ClientDC(),
		"coordinatorId": currentEngine.CoordinatorID(),
		"wanConnected":  currentEngine.WANConnected(),
	}
}
