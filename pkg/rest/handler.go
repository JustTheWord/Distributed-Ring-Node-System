package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitlab.fel.cvut.cz/B241_B2M32DSVA/grebegor/pkg/ringnode"
	"gitlab.fel.cvut.cz/B241_B2M32DSVA/grebegor/pkg/utils"
)

// NodeAPI - provides REST endpoints for a Node
type NodeAPI struct {
	Node *ringnode.Node
}

// JoinHandler - handle new node joining the ring
func (api *NodeAPI) JoinHandler(w http.ResponseWriter, r *http.Request) {
	var newNode ringnode.NeighborInfo
	if err := json.NewDecoder(r.Body).Decode(&newNode); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := api.Node.RingInsert(&newNode); err != nil {
		http.Error(w, fmt.Sprintf("Join failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"result":"ok"}`))
}

// LeaveHandler - gracefully leave the ring
func (api *NodeAPI) LeaveHandler(w http.ResponseWriter, r *http.Request) {
	if err := api.Node.RingLeave(); err != nil {
		http.Error(w, fmt.Sprintf("Leave failed: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"result":"left"}`))
}

// KillHandler - simulates a crash
func (api *NodeAPI) KillHandler(w http.ResponseWriter, r *http.Request) {
	// Force ring departure so other nodes link each other directly
	err := api.Node.RingLeave()
	if err != nil {
		utils.LogEvent(&api.Node.Clock, api.Node.NodeID,
			fmt.Sprintf("KillHandler: ring leave error: %v", err))
	}

	api.Node.StopGRPCServer()
	utils.LogEvent(&api.Node.Clock, api.Node.NodeID, "KillHandler: Node killed")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"result":"killed"}`))
}

// ReviveHandler - simulates revival
func (api *NodeAPI) ReviveHandler(w http.ResponseWriter, r *http.Request) {
	go func() {
		err := api.Node.StartGRPCServer()
		if err != nil {
			utils.LogEvent(&api.Node.Clock, api.Node.NodeID,
				fmt.Sprintf("ReviveHandler: failed to revive node: %v", err))
			return
		}
		utils.LogEvent(&api.Node.Clock, api.Node.NodeID, "ReviveHandler: gRPC server started")

		// Wait a short time, then re-insert into ring
		time.Sleep(1 * time.Second)
		err = doRingJoin("8081", api.Node.NodeID, api.Node.IP, api.Node.Port) // join via node2's REST
		if err != nil {
			utils.LogEvent(&api.Node.Clock, api.Node.NodeID,
				fmt.Sprintf("ReviveHandler: ring join failed: %v", err))
		} else {
			utils.LogEvent(&api.Node.Clock, api.Node.NodeID, "ReviveHandler: rejoined the ring")
		}
	}()

	utils.LogEvent(&api.Node.Clock, api.Node.NodeID, "ReviveHandler: Node revived")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"result":"revived"}`))
}

// doRingJoin calls POST /join to the given node's REST.
func doRingJoin(restPort string, nodeID string, ip string, port int32) error {
	url := fmt.Sprintf("http://127.0.0.1:%s/join", restPort)
	body := fmt.Sprintf(`{"node_id":"%s","ip":"%s","port":%d}`, nodeID, ip, port)

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ring join returned status %d", resp.StatusCode)
	}
	return nil
}

// EnterCSHandler - calls the coordinator logic if I'm the coordinator, else no-op.
func (api *NodeAPI) EnterCSHandler(w http.ResponseWriter, r *http.Request) {
	if api.Node.Coordinator != nil {
		granted, msg := api.Node.Coordinator.RequestAccess(api.Node.NodeID)
		if granted {
			api.Node.SharedVariable++
		}
		utils.LogEvent(&api.Node.Clock, api.Node.NodeID,
			fmt.Sprintf("EnterCS => granted=%t, msg=%s, new SharedVar=%d",
				granted, msg, api.Node.SharedVariable))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{"granted":%t,"msg":"%s"}`, granted, msg)))
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"granted":false,"msg":"Not coordinator (no forward logic)"}`))
	}
}

// LeaveCSHandler - calls the coordinator logic if I'm the coordinator, else no-op.
func (api *NodeAPI) LeaveCSHandler(w http.ResponseWriter, r *http.Request) {
	if api.Node.Coordinator != nil {
		success := api.Node.Coordinator.ReleaseAccess(api.Node.NodeID)
		utils.LogEvent(&api.Node.Clock, api.Node.NodeID,
			fmt.Sprintf("LeaveCS => success=%t", success))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{"success":%t}`, success)))
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":false,"msg":"Not coordinator (no forward logic)"}`))
	}
}

// StartSnapshotHandler - initiates a snapshot from this node (the aggregator).
func (api *NodeAPI) StartSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	snapshotID := fmt.Sprintf("snap-%d", utils.TimeNowMillis())

	api.Node.LockNode()
	ringSize := api.Node.RingSize
	api.Node.SnapshotMgr.SetExpectedResponses(snapshotID, ringSize)

	// Record local state right away
	localState := fmt.Sprintf("Node=%s, SharedVar=%d", api.Node.NodeID, api.Node.SharedVariable)
	api.Node.SnapshotData[snapshotID] = localState
	api.Node.SnapshotMgr.MarkRecorded(snapshotID)

	// aggregator = me
	api.Node.AggregatorID = api.Node.NodeID

	// Also add aggregator's local state
	api.Node.SnapshotMgr.AddRecordedState(snapshotID, localState)

	// Identify successor address *while still locked*:
	var succAddr string
	succ := api.Node.GetSuccessor() // <-- calls the new getter
	if succ != nil {
		succAddr = fmt.Sprintf("%s:%d", succ.Ip, succ.Port)
	}

	api.Node.UnlockNode()

	// Send the marker to successor, if any
	if succAddr != "" {
		go func() {
			_, err := ringnode.SendMarkerRPC(succAddr, &ringnode.MarkerMessage{
				SnapshotId:  snapshotID,
				InitiatorId: api.Node.NodeID,
			})
			if err != nil {
				utils.LogEvent(&api.Node.Clock, api.Node.NodeID,
					fmt.Sprintf("StartSnapshotHandler: Failed to send marker: %v", err))
			}
		}()
	}

	utils.LogEvent(&api.Node.Clock, api.Node.NodeID,
		fmt.Sprintf("StartSnapshotHandler: initiated snapshot=%s ringSize=%d", snapshotID, ringSize))

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"snapshotId":"%s"}`, snapshotID)))
}
