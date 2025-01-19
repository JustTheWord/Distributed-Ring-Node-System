# Distributed Ring Node System

A distributed system implementing a ring topology with gRPC communication, mutual exclusion, and snapshot (Chandy–Lamport) capabilities.

## Table of Contents

- [Introduction](#introduction)
- [Features](#features)
- [Architecture](#architecture)
- [Implementation Overview](#implementation-overview)
  - [Ring Topology & Membership](#ring-topology--membership)
  - [Coordinator-based Mutual Exclusion](#coordinator-based-mutual-exclusion)
  - [Chandy–Lamport Snapshot Algorithm](#chandy–lamport-snapshot-algorithm)
  - [REST Interface & Operations](#rest-interface--operations)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Usage](#usage)
  - [Starting Nodes](#starting-nodes)
  - [Interacting via REST API](#interacting-via-rest-api)
- [Logging](#logging)

## Introduction

The Distributed Ring Node System is a Go-based project that `implements a ring topology` for distributed systems. Each node communicates with its immediate successor and predecessor `using gRPC`, manages access to critical sections via mutual exclusion, and supports snapshotting `using the Chandy–Lamport algorithm`. A REST API is provided for external interactions, `allowing operations like joining the ring, entering/leaving critical sections, and initiating snapshots`.

## Features

- **Ring Management:** Dynamic joining and leaving of nodes in the ring, maintaining successor and predecessor relationships.
- **Mutual Exclusion:** Ensures exclusive access to critical sections using coordinator-based mutual exclusion.
- **Chandy–Lamport Snapshot:** Facilitates consistent global snapshots of the system state without halting operations.
- **REST API:** Provides HTTP endpoints for managing nodes and interacting with the system.
- **Logging:** Implements Lamport clocks for event ordering and logs events to both console and a log file.

## Architecture

![Architecture Diagram](docs/architecture.png)

The system consists of multiple ring nodes, each running as a separate instance. Each node exposes a gRPC server for peer-to-peer communication and a REST server for external HTTP interactions. Nodes communicate with each other via gRPC to manage ring membership, coordinate access to critical sections, and perform snapshot operations. A REST API is exposed for external interactions, allowing users to send HTTP requests to perform various operations. Node1 may act as the coordinator if started with `isCoordinator=true`.

## Implementation Overview

### Ring Topology & Membership

Each node holds:

- A pointer/reference to its predecessor and successor in the ring (`n.predecessor`, `n.successor`).
- Methods to insert a new node between itself and its current successor, or to gracefully leave the ring.
- gRPC endpoints (`InformSuccessor`, `InformPredecessor`) to update ring links.

**Joining the Ring:**

When a node A receives a request to join from node B:

1. B is inserted as A's successor.
2. A notifies its old successor to set B as predecessor.
3. B sets A as its predecessor.

**Leaving the Ring:**

When a node leaves:

1. It connects its predecessor and successor together.
2. Updates the ring size accordingly.

### Coordinator-based Mutual Exclusion

One node can be started with `isCoordinator=true`, making it the single coordinator in the ring. This node holds an instance of Coordinator, which:

- Tracks the lock status (`isLocked`) and who currently holds it (`currentHolder`).
- Maintains a FIFO queue of waiting nodes (`requestQueue`).

**Accessing Critical Sections:**

1. Nodes call the REST endpoint `/enterCS`, which internally calls the coordinator’s gRPC method `RequestAccess(nodeID)`.
2. If access is granted, the node increments a `SharedVariable` or performs its critical-section work.
3. Upon calling `/leaveCS`, the node calls `ReleaseAccess(nodeID)`, and the coordinator grants access to the next node in the queue.

### Chandy–Lamport Snapshot Algorithm

Each node integrates a `ChandyLamportManager` for snapshot coordination:

1. **Initiating a Snapshot:**

   - A node initiating a snapshot becomes the aggregator.
   - It records its local state immediately and triggers a marker message to its successor.

2. **Receiving a Marker:**

   - Upon receiving a marker for the first time:
     - The node records its local state (e.g., the `SharedVariable`, node ID).
     - Forwards the marker to its successor.
     - If it’s not the aggregator, it sends its recorded state back to the aggregator via the `SendRecordedState` gRPC call.

3. **Completing the Snapshot:**
   - The aggregator collects recorded states from all nodes.
   - Once all states are collected, the snapshot is complete.

### REST Interface & Operations

Each node runs an HTTP server (default ports 8080, 8081, 8082, etc.). The main endpoints are:

- **POST /join:** Join the ring. The body contains `{node_id, ip, port}` of the joining node.
- **POST /leave:** Gracefully leave the ring, updating predecessor and successor accordingly.
- **POST /kill:** Simulate a crash (stop the node’s gRPC server; ring repairs itself).
- **POST /revive:** Simulate revival (restart the node’s gRPC server; rejoin the ring).
- **POST /enterCS:** Request to enter the critical section (only meaningful if you are the coordinator or you forward the request).
- **POST /leaveCS:** Release the critical section.
- **POST /startSnapshot:** Initiate a Chandy–Lamport snapshot from this node, which will become the aggregator.

## Prerequisites

- **Go:** Ensure that Go is installed on your system. You can download it from [https://golang.org/dl/](https://golang.org/dl/).
- **Protocol Buffers:** Required for gRPC. Install the `protoc` compiler.

  ```bash
  # For macOS using Homebrew
  brew install protobuf

  # For Linux, Debian
  sudo apt install -y protobuf-compiler
  ```

- **gRPC Go Plugin:** Required to generate Go code from .proto files

  ```bash
  go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.3
  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.4.7
  ```

## Installation

1. **Clone the repository**:

   ```bash
   git clone git@gitlab.fel.cvut.cz:B241_B2M32DSVA/grebegor.git
   ```

2. **Generate gRPC Code: Ensure that the .proto files are correctly defined with the go_package option**:

   ```bash
   protoc --go_out=. --go-grpc_out=. pkg/ringnode/ringnode.proto
   ```

3. **Build the Project**:

   ```bash
    go build -o ringnode ./cmd/main.go
   ```

## Usage

Simulate multiple nodes in your ring to test functionalities like joining, critical section access, and snapshot initiation.

### Starting Nodes

Example: Start 3 nodes with IDs 1, 2, and 3:

| Node ID | IP Address | Port | Start as Coordinator | REST API Port |
| ------- | ---------- | ---- | -------------------- | ------------- |
| node1   | 127.0.0.1  | 5001 | true                 | 8080          |
| node2   | 127.0.0.1  | 5002 | false                | 8081          |
| node3   | 127.0.0.1  | 5003 | false                | 8082          |

```bash
go run ./cmd/main.go node1 127.0.0.1 5001 true 8080
go run ./cmd/main.go node2 127.0.0.1 5002 false 8081
go run ./cmd/main.go node3 127.0.0.1 5003 false 8082
```

### Interacting via REST API

Use tools like curl, Postman, or any REST client to interact with the REST endpoints.

#### Join a Node

Join `node2` to `node1` (if `node2` wasn’t started in the ring yet):

```bash
curl -X POST -H "Content-Type: application/json" \
-d '{"node_id":"node2","ip":"127.0.0.1","port":5002}' \
http://127.0.0.1:8080/join
```

#### Enter Critical Section

From `node1`, if it is the coordinator:

```bash
curl -X POST http://127.0.0.1:8080/enterCS
```

This calls the `coordinator.RequestAccess(nodeID)` method.

#### Leave Critical Section

```bash
curl -X POST http://127.0.0.1:8080/leaveCS
```

#### Start Snapshot

Initiate a Chandy–Lamport snapshot from `node1`:

```bash
curl -X POST http://127.0.0.1:8080/startSnapshot
```

#### Kill a Node

Simulate killing `node2`:

```bash
curl -X POST http://127.0.0.1:8081/kill
```

#### Revive a Node

Simulate reviving `node2`:

```bash
curl -X POST http://127.0.0.1:8081/revive
```

## Logging

The system uses Lamport clocks for event ordering. Events are logged to both the console and a `distributed.log` file located in the root directory. This log includes timestamps, node IDs, and event descriptions, facilitating easier debugging and monitoring.

Example Log Entry:

```
2025-01-17T22:46:27+01:00 [1] [node1] JoinHandler: Node node2 joined as successor
2025-01-17T22:46:55+01:00 [2] [node1] EnterCS: node1 => granted=true; Granted; new SharedVar=1
2025-01-17T22:47:12+01:00 [1] [node2] Received Marker for snapshot snap-1737150432950 from node1
2025-01-17T22:48:16+01:00 [3] [node1] KillHandler: Node killed
2025-01-17T22:48:49+01:00 [4] [node1] ReviveHandler: Node revived
```
