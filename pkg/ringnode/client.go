package ringnode

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
)

// InformSuccessor sends the InformSuccessor RPC to the specified node address.
func InformSuccessor(nodeAddr string, info *NeighborInfo) (*Ack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, nodeAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", nodeAddr, err)
	}
	defer conn.Close()

	client := NewRingNodeClient(conn)

	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer rpcCancel()

	ack, err := client.InformSuccessor(rpcCtx, info)
	if err != nil {
		return nil, fmt.Errorf("InformSuccessor RPC failed: %v", err)
	}
	return ack, nil
}

// InformPredecessor sends the InformPredecessor RPC to the specified node address.
func InformPredecessor(nodeAddr string, info *NeighborInfo) (*Ack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, nodeAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", nodeAddr, err)
	}
	defer conn.Close()

	client := NewRingNodeClient(conn)

	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer rpcCancel()

	ack, err := client.InformPredecessor(rpcCtx, info)
	if err != nil {
		return nil, fmt.Errorf("InformPredecessor RPC failed: %v", err)
	}
	return ack, nil
}

// SendMarkerRPC is a convenience function for calling the SendMarker gRPC.
func SendMarkerRPC(nodeAddr string, marker *MarkerMessage) (*Ack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, nodeAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", nodeAddr, err)
	}
	defer conn.Close()

	client := NewRingNodeClient(conn)

	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer rpcCancel()

	ack, err := client.SendMarker(rpcCtx, marker)
	if err != nil {
		return nil, fmt.Errorf("SendMarker RPC failed: %v", err)
	}
	return ack, nil
}

// SendRecordedStateRPC sends the local recorded state to the aggregator.
func SendRecordedStateRPC(nodeAddr string, rec *RecordedState) (*Ack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, nodeAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to aggregator at %s: %v", nodeAddr, err)
	}
	defer conn.Close()

	client := NewRingNodeClient(conn)

	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer rpcCancel()

	return client.SendRecordedState(rpcCtx, rec)
}
