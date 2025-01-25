package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"gitlab.fel.cvut.cz/B241_B2M32DSVA/grebegor/pkg/rest"
	"gitlab.fel.cvut.cz/B241_B2M32DSVA/grebegor/pkg/ringnode"
	"gitlab.fel.cvut.cz/B241_B2M32DSVA/grebegor/pkg/snapshot"
)

func main() {
	// Now we allow up to 7 args:
	// Usage: main.go <nodeID> <ip> <gRPC port> <isCoordinator> [<restPort>] [<coordinatorGrpcAddr>] [<coordinatorRestPort>]
	if len(os.Args) < 5 {
		fmt.Println("Usage: main.go <nodeID> <ip> <gRPC port> <isCoordinator> [<restPort>] [<coordGrpcAddr>] [<coordRestPort>]")
		return
	}

	nodeID := os.Args[1]
	ip := os.Args[2]
	grpcPort, _ := strconv.Atoi(os.Args[3])
	isCoordinator := (os.Args[4] == "true")

	// Default REST port
	restPort := "8080"
	if len(os.Args) > 5 {
		restPort = os.Args[5]
	}

	// If not coordinator, we can optionally get a gRPC address for the real coordinator
	coordinatorGrpcAddr := ""
	if !isCoordinator && len(os.Args) > 6 {
		coordinatorGrpcAddr = os.Args[6]
	}

	// OPTIONAL: also get the coordinator's REST port if we want to rejoin automatically
	coordinatorRestPort := ""
	if !isCoordinator && len(os.Args) > 7 {
		coordinatorRestPort = os.Args[7]
	}

	// Create the Node
	node := ringnode.NewNode(nodeID, ip, int32(grpcPort), isCoordinator)

	// Store known coordinator addresses (both gRPC and REST)
	node.CoordinatorAddr = coordinatorGrpcAddr     // e.g. "127.0.0.1:5001"
	node.CoordinatorRestPort = coordinatorRestPort // e.g. "8001"

	// Initialize the Chandy-Lamport Manager (hard-coded ring size = 5 for demo)
	node.SnapshotMgr = snapshot.NewChandyLamportManager(5)

	// Start gRPC server in a goroutine
	go func() {
		err := node.StartGRPCServer()
		if err != nil {
			log.Fatal("Failed to start gRPC server:", err)
		}
	}()

	// Setup REST handlers
	api := &rest.NodeAPI{Node: node}
	http.HandleFunc("/join", api.JoinHandler)
	http.HandleFunc("/leave", api.LeaveHandler)
	http.HandleFunc("/kill", api.KillHandler)
	http.HandleFunc("/revive", api.ReviveHandler)

	// Mutual exclusion
	http.HandleFunc("/enterCS", api.EnterCSHandler)
	http.HandleFunc("/leaveCS", api.LeaveCSHandler)

	// Snapshot
	http.HandleFunc("/startSnapshot", api.StartSnapshotHandler)

	// If you want a /sharedVar endpoint:
	http.HandleFunc("/sharedVar", api.GetSharedVarHandler)

	log.Printf("Node %s running. gRPC on %d, REST on %s\n", nodeID, grpcPort, restPort)
	log.Fatal(http.ListenAndServe(":"+restPort, nil))
}
