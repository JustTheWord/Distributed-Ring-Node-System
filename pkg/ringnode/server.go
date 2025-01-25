package ringnode

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"gitlab.fel.cvut.cz/B241_B2M32DSVA/grebegor/pkg/concurrency"
	"gitlab.fel.cvut.cz/B241_B2M32DSVA/grebegor/pkg/snapshot"
	"gitlab.fel.cvut.cz/B241_B2M32DSVA/grebegor/pkg/utils"
	"google.golang.org/grpc"
)

const (
	// Max 500ms random delay.
	MaxDelayMs = 500
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

	// If not coordinator, we can store the coordinator's addresses:
	CoordinatorAddr     string // gRPC address, e.g. "127.0.0.1:5001"
	CoordinatorRestPort string // REST port, e.g. "8001"

	// The ring size
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
	rand.Seed(time.Now().UnixNano()) // seed for random delays
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

// GetSuccessorAddress is used by other logic
func (n *Node) GetSuccessorAddress() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.successor == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", n.successor.Ip, n.successor.Port)
}

// Helper to look up an address by NodeID (in ring predecessor/successor only).
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

// maybeDelay injects an artificial random delay for concurrency testing
func (n *Node) maybeDelay() {
	d := rand.Intn(MaxDelayMs) // up to MaxDelayMs
	time.Sleep(time.Duration(d) * time.Millisecond)
}

// ----------------------------------------------------------------------------
// RPC methods for ringnode.RingNodeServer interface
// ----------------------------------------------------------------------------

// InformSuccessor sets the node’s successor pointer.
func (n *Node) InformSuccessor(ctx context.Context, info *NeighborInfo) (*Ack, error) {
	// 1) artificial delay
	n.maybeDelay()

	n.mu.Lock()
	defer n.mu.Unlock()

	n.successor = info
	utils.LogEvent(&n.Clock, n.NodeID,
		fmt.Sprintf("Set successor to %s (%s:%d)", info.NodeId, info.Ip, info.Port))

	return &Ack{Success: true, Message: "Successor updated"}, nil
}

// InformPredecessor sets the node’s predecessor pointer.
func (n *Node) InformPredecessor(ctx context.Context, info *NeighborInfo) (*Ack, error) {
	// 1) artificial delay
	n.maybeDelay()

	n.mu.Lock()
	defer n.mu.Unlock()

	n.predecessor = info
	utils.LogEvent(&n.Clock, n.NodeID,
		fmt.Sprintf("Set predecessor to %s (%s:%d)", info.NodeId, info.Ip, info.Port))

	return &Ack{Success: true, Message: "Predecessor updated"}, nil
}

// SendMarker is called by the predecessor with a "marker" message (Chandy–Lamport).
func (n *Node) SendMarker(ctx context.Context, marker *MarkerMessage) (*Ack, error) {
	// 1) artificial delay
	n.maybeDelay()

	n.mu.Lock()
	defer n.mu.Unlock()

	snapID := marker.SnapshotId
	utils.LogEvent(&n.Clock, n.NodeID,
		fmt.Sprintf("Received marker for snapshot=%s from initiator=%s", snapID, marker.InitiatorId))

	if !n.SnapshotMgr.HasRecorded(snapID) {
		// Record local state
		localState := fmt.Sprintf("Node=%s, SharedVar=%d", n.NodeID, n.SharedVariable)
		n.SnapshotData[snapID] = localState
		n.SnapshotMgr.MarkRecorded(snapID)

		// If aggregator is not me, send local state to aggregator
		if n.AggregatorID != "" && n.AggregatorID != n.NodeID {
			aggAddr := n.getNodeAddrByID(n.AggregatorID)
			if aggAddr != "" {
				go func(agg, rec string) {
					_, err := SendRecordedStateRPC(agg, &RecordedState{
						NodeId:       n.NodeID,
						SnapshotId:   snapID,
						LocalState:   rec,
						ChannelState: "",
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
		if n.NodeID == n.AggregatorID {
			if n.SnapshotMgr.IsComplete(snapID) {
				utils.LogEvent(&n.Clock, n.NodeID,
					fmt.Sprintf("Snapshot %s complete! States: %v",
						snapID, n.SnapshotMgr.GetAllStates(snapID)))
			}
		}
	}

	return &Ack{Success: true, Message: "Marker processed"}, nil
}

// SendRecordedState is used if we forward local states to an aggregator node.
func (n *Node) SendRecordedState(ctx context.Context, rec *RecordedState) (*Ack, error) {
	// 1) artificial delay
	n.maybeDelay()

	utils.LogEvent(&n.Clock, n.NodeID,
		fmt.Sprintf("Received recorded state from %s for snapshot=%s", rec.NodeId, rec.SnapshotId))

	if n.NodeID == n.AggregatorID {
		count := n.SnapshotMgr.AddRecordedState(rec.SnapshotId, rec.LocalState)
		utils.LogEvent(&n.Clock, n.NodeID,
			fmt.Sprintf("Aggregator: snapshot=%s from %s => stored. total so far=%d",
				rec.SnapshotId, rec.NodeId, count))

		if n.SnapshotMgr.IsComplete(rec.SnapshotId) {
			final := n.SnapshotMgr.GetAllStates(rec.SnapshotId)
			utils.LogEvent(&n.Clock, n.NodeID,
				fmt.Sprintf("Snapshot %s complete, final states: %v", rec.SnapshotId, final))
		}
	}
	return &Ack{Success: true, Message: "Recorded state received"}, nil
}

// SendData would handle normal ring passing of some application data
func (n *Node) SendData(ctx context.Context, data *DataMessage) (*Ack, error) {
	// artificial delay
	n.maybeDelay()

	utils.LogEvent(&n.Clock, n.NodeID,
		fmt.Sprintf("Received data from %s: %s", data.FromNodeId, data.Payload))
	return &Ack{Success: true, Message: "Data received"}, nil
}

// RequestAccess is the RPC for mutual-exclusion requests
func (n *Node) RequestAccess(ctx context.Context, req *MutualExclusionRequest) (*MutualExclusionResponse, error) {
	// artificial delay
	n.maybeDelay()

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.Coordinator != nil {
		granted, msg := n.Coordinator.RequestAccess(req.NodeId)

		if granted {
			n.SharedVariable++
		}

		utils.LogEvent(&n.Clock, n.NodeID,
			fmt.Sprintf("Coordinator: RequestAccess from %s => granted=%t, msg=%s, new SharedVar=%d",
				req.NodeId, granted, msg, n.SharedVariable))

		return &MutualExclusionResponse{Granted: granted, Message: msg}, nil
	}
	utils.LogEvent(&n.Clock, n.NodeID, "Ignoring RequestAccess - not coordinator.")
	return &MutualExclusionResponse{Granted: false, Message: "Not coordinator"}, nil
}

// ReleaseAccess is the RPC for mutual-exclusion release
func (n *Node) ReleaseAccess(ctx context.Context, rel *MutualExclusionRelease) (*Ack, error) {
	// artificial delay
	n.maybeDelay()

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
