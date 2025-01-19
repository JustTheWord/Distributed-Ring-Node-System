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
	if len(os.Args) < 5 {
		fmt.Println("Usage: main.go <nodeID> <ip> <port> <isCoordinator> <restPort>")
		return
	}
	nodeID := os.Args[1]
	ip := os.Args[2]
	port, _ := strconv.Atoi(os.Args[3])
	isCoordinator := os.Args[4] == "true"
	restPort := "8080"
	if len(os.Args) > 5 {
		restPort = os.Args[5]
	}

	node := ringnode.NewNode(nodeID, ip, int32(port), isCoordinator)
	// Hard-coded "maximum" ring size, for demonstration:
	node.SnapshotMgr = snapshot.NewChandyLamportManager(5)

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

	log.Printf("Node %s running. gRPC on %d, REST on %s\n", nodeID, port, restPort)
	log.Fatal(http.ListenAndServe(":"+restPort, nil))
}
