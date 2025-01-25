package ringnode

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
)

// NewRingNodeClientGRPC is a helper to connect to a ringnode server and return the client + conn.
func NewRingNodeClientGRPC(targetAddr string) (RingNodeClient, *grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, targetAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to %s: %v", targetAddr, err)
	}
	client := NewRingNodeClient(conn)
	return client, conn, nil
}

// InformSuccessor ...
func InformSuccessor(nodeAddr string, info *NeighborInfo) (*Ack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, nodeAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", nodeAddr, err)
	}
	defer conn.Close()

	client := NewRingNodeClient(conn)
	return client.InformSuccessor(ctx, info)
}

// InformPredecessor ...
func InformPredecessor(nodeAddr string, info *NeighborInfo) (*Ack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, nodeAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", nodeAddr, err)
	}
	defer conn.Close()

	client := NewRingNodeClient(conn)
	return client.InformPredecessor(ctx, info)
}

// SendMarkerRPC ...
func SendMarkerRPC(nodeAddr string, marker *MarkerMessage) (*Ack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, nodeAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", nodeAddr, err)
	}
	defer conn.Close()

	client := NewRingNodeClient(conn)
	return client.SendMarker(ctx, marker)
}

// SendRecordedStateRPC ...
func SendRecordedStateRPC(nodeAddr string, rec *RecordedState) (*Ack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, nodeAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to aggregator at %s: %v", nodeAddr, err)
	}
	defer conn.Close()

	client := NewRingNodeClient(conn)
	return client.SendRecordedState(ctx, rec)
}
