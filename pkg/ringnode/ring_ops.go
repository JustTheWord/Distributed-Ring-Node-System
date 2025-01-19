package ringnode

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
)

// RingInsert inserts a new node (newInfo) into my ring, placing it between me and my old successor.
func (n *Node) RingInsert(newInfo *NeighborInfo) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// If I'm alone in the ring:
	if n.predecessor == nil && n.successor == nil {
		n.successor = newInfo
		n.predecessor = newInfo

		myInfo := &NeighborInfo{
			NodeId: n.NodeID,
			Ip:     n.IP,
			Port:   n.Port,
		}

		nodeAddr := fmt.Sprintf("%s:%d", newInfo.Ip, newInfo.Port)
		if err := informSuccessorRPC(nodeAddr, myInfo); err != nil {
			return err
		}
		if err := informPredecessorRPC(nodeAddr, myInfo); err != nil {
			return err
		}

		n.RingSize = 2
		return nil
	}

	// Insert new node between me and my current successor
	oldSucc := n.successor
	n.successor = newInfo

	myInfo := &NeighborInfo{
		NodeId: n.NodeID,
		Ip:     n.IP,
		Port:   n.Port,
	}
	nodeAddr := fmt.Sprintf("%s:%d", newInfo.Ip, newInfo.Port)
	if err := informPredecessorRPC(nodeAddr, myInfo); err != nil {
		return err
	}
	if oldSucc != nil {
		if err := informSuccessorRPC(nodeAddr, oldSucc); err != nil {
			return err
		}
		oldSuccAddr := fmt.Sprintf("%s:%d", oldSucc.Ip, oldSucc.Port)
		if err := informPredecessorRPC(oldSuccAddr, newInfo); err != nil {
			return err
		}
	}

	n.RingSize++ // simplistic approach
	return nil
}

// RingLeave tries to remove me from the ring gracefully, hooking predecessor <-> successor.
func (n *Node) RingLeave() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.predecessor == nil && n.successor == nil {
		// I'm alone in the ring. Nothing to fix.
		return nil
	}

	if n.predecessor != nil && n.successor != nil {
		predAddr := fmt.Sprintf("%s:%d", n.predecessor.Ip, n.predecessor.Port)
		if err := informSuccessorRPC(predAddr, n.successor); err != nil {
			return err
		}

		succAddr := fmt.Sprintf("%s:%d", n.successor.Ip, n.successor.Port)
		if err := informPredecessorRPC(succAddr, n.predecessor); err != nil {
			return err
		}
	}

	n.successor = nil
	n.predecessor = nil

	if n.RingSize > 1 {
		n.RingSize--
	}

	return nil
}

func informSuccessorRPC(nodeAddr string, info *NeighborInfo) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, nodeAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("informSuccessorRPC: dial %s failed: %v", nodeAddr, err)
	}
	defer conn.Close()

	client := NewRingNodeClient(conn)
	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer rpcCancel()

	_, err = client.InformSuccessor(rpcCtx, info)
	return err
}

func informPredecessorRPC(nodeAddr string, info *NeighborInfo) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, nodeAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("informPredecessorRPC: dial %s failed: %v", nodeAddr, err)
	}
	defer conn.Close()

	client := NewRingNodeClient(conn)
	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer rpcCancel()

	_, err = client.InformPredecessor(rpcCtx, info)
	return err
}
