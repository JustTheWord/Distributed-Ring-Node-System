package ringnode

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"

	"gitlab.fel.cvut.cz/B241_B2M32DSVA/grebegor/pkg/concurrency"
	"gitlab.fel.cvut.cz/B241_B2M32DSVA/grebegor/pkg/snapshot"
	"gitlab.fel.cvut.cz/B241_B2M32DSVA/grebegor/pkg/utils"
	"google.golang.org/grpc"
)

// Node implements the RingNodeServer interface.
type Node struct {
	UnimplementedRingNodeServer

	NodeID string
	IP     string
	Port   int32

	successor   *NeighborInfo
	predecessor *NeighborInfo

	mu    sync.Mutex
	Clock int

	IsCoordinator bool
	Coordinator   *concurrency.Coordinator

	// The ring size (simple approach)
	RingSize int

	// A simple shared variable
	SharedVariable int

	// Snapshot manager
	SnapshotData map[string]string
	SnapshotMgr  *snapshot.ChandyLamportManager

	// The node that started the snapshot
	AggregatorID string

	// channelStates[snapshotID] => slice of messages that arrived before marker
	channelStates map[string][]string

	grpcServer *grpc.Server
}

// NewNode constructs a Node.
func NewNode(nodeID, ip string, port int32, isCoordinator bool) *Node {
	var coord *concurrency.Coordinator
	if isCoordinator {
		coord = concurrency.NewCoordinator()
	}
	return &Node{
		NodeID:        nodeID,
		IP:            ip,
		Port:          port,
		IsCoordinator: isCoordinator,
		Coordinator:   coord,
		SnapshotData:  make(map[string]string),
		channelStates: make(map[string][]string),
		RingSize:      1, // at least ourselves
	}
}

func (n *Node) LockNode()   { n.mu.Lock() }
func (n *Node) UnlockNode() { n.mu.Unlock() }

func (n *Node) GetSuccessor() *NeighborInfo {
	return n.successor
}

func (n *Node) StartGRPCServer() error {
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", n.IP, n.Port))
	if err != nil {
		return fmt.Errorf("failed to listen on %s:%d: %v", n.IP, n.Port, err)
	}

	s := grpc.NewServer()
	RegisterRingNodeServer(s, n)
	n.grpcServer = s
	log.Printf("[Node %s] Starting gRPC server on %s:%d\n", n.NodeID, n.IP, n.Port)

	return s.Serve(lis)
}

func (n *Node) StopGRPCServer() {
	if n.grpcServer != nil {
		log.Printf("[Node %s] Stopping gRPC server.\n", n.NodeID)
		n.grpcServer.GracefulStop()
		n.grpcServer = nil
	}
}

// GetSuccessorAddress is used by other logic (not by /startSnapshot anymore) to find ring successor
func (n *Node) GetSuccessorAddress() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.successor == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", n.successor.Ip, n.successor.Port)
}

// A small helper to get address from NodeID in a ring of successor/predecessor only.
// In a more robust design, you'd keep a map of NodeID -> ip:port, or some DHT/routing structure.
func (n *Node) getNodeAddrByID(targetID string) string {
	n.mu.Lock()
	defer n.mu.Unlock()

	if targetID == n.NodeID {
		return fmt.Sprintf("%s:%d", n.IP, n.Port)
	}
	if n.successor != nil && n.successor.NodeId == targetID {
		return fmt.Sprintf("%s:%d", n.successor.Ip, n.successor.Port)
	}
	if n.predecessor != nil && n.predecessor.NodeId == targetID {
		return fmt.Sprintf("%s:%d", n.predecessor.Ip, n.predecessor.Port)
	}
	return ""
}

// ----------------------------------------------------------------------------
// RPC methods for ringnode.RingNodeServer interface
// ----------------------------------------------------------------------------

// InformSuccessor sets the node’s successor pointer.
func (n *Node) InformSuccessor(ctx context.Context, info *NeighborInfo) (*Ack, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.successor = info

	utils.LogEvent(&n.Clock, n.NodeID,
		fmt.Sprintf("Set successor to %s (%s:%d)", info.NodeId, info.Ip, info.Port))

	return &Ack{Success: true, Message: "Successor updated"}, nil
}

// InformPredecessor sets the node’s predecessor pointer.
func (n *Node) InformPredecessor(ctx context.Context, info *NeighborInfo) (*Ack, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.predecessor = info

	utils.LogEvent(&n.Clock, n.NodeID,
		fmt.Sprintf("Set predecessor to %s (%s:%d)", info.NodeId, info.Ip, info.Port))

	return &Ack{Success: true, Message: "Predecessor updated"}, nil
}

// SendMarker is called by the predecessor with a "marker" message.
// If we haven't recorded for this snapshot, do so, then forward marker to successor.
// If we already recorded, and we're the aggregator, check if snapshot is complete.
func (n *Node) SendMarker(ctx context.Context, marker *MarkerMessage) (*Ack, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	snapID := marker.SnapshotId
	utils.LogEvent(&n.Clock, n.NodeID,
		fmt.Sprintf("Received marker for snapshot=%s from initiator=%s", snapID, marker.InitiatorId))

	// If not yet recorded for this snapshot
	if !n.SnapshotMgr.HasRecorded(snapID) {
		// Record local state
		localState := fmt.Sprintf("Node=%s, SharedVar=%d", n.NodeID, n.SharedVariable)
		n.SnapshotData[snapID] = localState
		n.SnapshotMgr.MarkRecorded(snapID)

		// If aggregator is not me, send local state to aggregator
		if n.AggregatorID != "" && n.AggregatorID != n.NodeID {
			aggAddr := n.getNodeAddrByID(n.AggregatorID)
			if aggAddr != "" {
				go func(agg string, rec string) {
					_, err := SendRecordedStateRPC(agg, &RecordedState{
						NodeId:       n.NodeID,
						SnapshotId:   snapID,
						LocalState:   rec,
						ChannelState: "", // omitted for brevity
					})
					if err != nil {
						utils.LogEvent(&n.Clock, n.NodeID,
							fmt.Sprintf("Error sending state to aggregator: %v", err))
					}
				}(aggAddr, localState)
			}
		}

		// Forward marker to my successor
		if n.successor != nil {
			succAddr := fmt.Sprintf("%s:%d", n.successor.Ip, n.successor.Port)
			go func(addr string, msg *MarkerMessage) {
				_, err := SendMarkerRPC(addr, msg)
				if err != nil {
					utils.LogEvent(&n.Clock, n.NodeID,
						fmt.Sprintf("Failed forwarding marker: %v", err))
				}
			}(succAddr, marker)
		}
	} else {
		// Already recorded
		// If aggregator == me, see if snapshot is now complete
		if n.NodeID == n.AggregatorID {
			if n.SnapshotMgr.IsComplete(snapID) {
				utils.LogEvent(&n.Clock, n.NodeID,
					fmt.Sprintf("Snapshot %s complete! States: %v",
						snapID, n.SnapshotMgr.GetAllStates(snapID)))
			}
		}
		// else do nothing (marker traveling around again).
	}

	return &Ack{Success: true, Message: "Marker processed"}, nil
}

// SendRecordedState is used if we forward local states to an aggregator node.
func (n *Node) SendRecordedState(ctx context.Context, rec *RecordedState) (*Ack, error) {
	utils.LogEvent(&n.Clock, n.NodeID,
		fmt.Sprintf("Received recorded state from %s for snapshot=%s", rec.NodeId, rec.SnapshotId))

	// If we are the aggregator, store the state
	if n.NodeID == n.AggregatorID {
		count := n.SnapshotMgr.AddRecordedState(rec.SnapshotId, rec.LocalState)
		utils.LogEvent(&n.Clock, n.NodeID,
			fmt.Sprintf("Aggregator: snapshot=%s from %s => stored. total so far=%d",
				rec.SnapshotId, rec.NodeId, count))

		// Check if we have them all
		if n.SnapshotMgr.IsComplete(rec.SnapshotId) {
			final := n.SnapshotMgr.GetAllStates(rec.SnapshotId)
			utils.LogEvent(&n.Clock, n.NodeID,
				fmt.Sprintf("Snapshot %s complete, final states: %v", rec.SnapshotId, final))
		}
	}
	// else if we're not aggregator, we ignore.

	return &Ack{Success: true, Message: "Recorded state received"}, nil
}

// SendData would handle normal ring passing of data
func (n *Node) SendData(ctx context.Context, data *DataMessage) (*Ack, error) {
	utils.LogEvent(&n.Clock, n.NodeID,
		fmt.Sprintf("Received data from %s: %s", data.FromNodeId, data.Payload))
	return &Ack{Success: true, Message: "Data received"}, nil
}

// RequestAccess is the RPC for mutual-exclusion requests
func (n *Node) RequestAccess(ctx context.Context, req *MutualExclusionRequest) (*MutualExclusionResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.Coordinator != nil {
		granted, msg := n.Coordinator.RequestAccess(req.NodeId)
		utils.LogEvent(&n.Clock, n.NodeID,
			fmt.Sprintf("Coordinator: RequestAccess from %s => granted=%t, msg=%s", req.NodeId, granted, msg))
		return &MutualExclusionResponse{Granted: granted, Message: msg}, nil
	}
	utils.LogEvent(&n.Clock, n.NodeID, "Ignoring RequestAccess - not coordinator.")
	return &MutualExclusionResponse{Granted: false, Message: "Not coordinator"}, nil
}

// ReleaseAccess is the RPC for mutual-exclusion release
func (n *Node) ReleaseAccess(ctx context.Context, rel *MutualExclusionRelease) (*Ack, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.Coordinator != nil {
		success := n.Coordinator.ReleaseAccess(rel.NodeId)
		utils.LogEvent(&n.Clock, n.NodeID,
			fmt.Sprintf("Coordinator: ReleaseAccess from %s => success=%t", rel.NodeId, success))
		return &Ack{Success: success, Message: "Released"}, nil
	}
	utils.LogEvent(&n.Clock, n.NodeID, "Ignoring ReleaseAccess - not coordinator.")
	return &Ack{Success: false, Message: "Not coordinator"}, nil
}
