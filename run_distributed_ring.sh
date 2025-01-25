#!/bin/bash

# run_distributed_ring.sh
# Enhanced script to set up and test a distributed ring system with integrated scenarios.

# Exit immediately if a command exits with a non-zero status
set -e

# Default configurations
DEFAULT_NUM_NODES=5
DEFAULT_OPERATION_DURATION=60 # in seconds
LOG_DIR="logs"
OPERATION_LOG="$LOG_DIR/operations.log"
CENTRAL_LOG="$LOG_DIR/central.log"
SCENARIO="basic"

# Ensure log directory exists
mkdir -p "$LOG_DIR"

# Function to display usage
usage() {
  echo "Usage: $0 [OPTIONS]"
  echo "Options:"
  echo "  -n, --nodes        Number of nodes to start (minimum 1, default: $DEFAULT_NUM_NODES)"
  echo "  -s, --scenario     Scenario to execute (default: basic)"
  echo "                     Available scenarios:"
  echo "                       basic                   - Basic Operation Sequence"
  echo "                       fault_tolerance         - Fault Tolerance Testing"
  echo "                       dynamic_membership      - Dynamic Membership Changes"
  echo "                       coordinator_forwarding  - Test coordinator forwarding logic"
  echo "  -c, --config       Path to configuration file for operations (future feature)"
  echo "  -h, --help         Display this help message"
  exit 1
}

# Parse command-line arguments
NUM_NODES=$DEFAULT_NUM_NODES
CONFIG_FILE=""
SCENARIO="basic"

# Use getopt for parsing
PARSED_ARGS=$(getopt -o n:s:c:h --long nodes:,scenario:,config:,help -- "$@")
if [[ $? -ne 0 ]]; then
  usage
fi

eval set -- "$PARSED_ARGS"

while true; do
  case "$1" in
  -n | --nodes)
    NUM_NODES="$2"
    shift 2
    ;;
  -s | --scenario)
    SCENARIO="$2"
    shift 2
    ;;
  -c | --config)
    CONFIG_FILE="$2"
    shift 2
    ;;
  -h | --help)
    usage
    ;;
  --)
    shift
    break
    ;;
  *)
    usage
    ;;
  esac
done

echo "Starting distributed ring with $NUM_NODES node(s)."

# Define base ports
BASE_GRPC_PORT=5000
BASE_REST_PORT=8000

# Arrays to hold node configurations
NODE_IDS=()
NODE_IPS=()
NODE_PORTS=()
NODE_REST_PORTS=()
NODE_COORDINATORS=()

# ------------------------------------------------------------------------------
# 1) We initialize the arrays with up to NUM_NODES. 
#    But note that for the "dynamic_membership" scenario, we will 
#    RE-INIT these arrays inside its function to ensure we only
#    start node1..node3 initially. 
# ------------------------------------------------------------------------------

for ((i = 1; i <= NUM_NODES; i++)); do
  NODE_ID="node$i"
  NODE_IP="127.0.0.1"
  NODE_PORT=$((BASE_GRPC_PORT + i))
  NODE_REST_PORT=$((BASE_REST_PORT + i))

  NODE_IDS+=("$NODE_ID")
  NODE_IPS+=("$NODE_IP")
  NODE_PORTS+=("$NODE_PORT")
  NODE_REST_PORTS+=("$NODE_REST_PORT")

  # First node is coordinator
  if [ "$i" -eq 1 ]; then
    NODE_COORDINATORS+=(true)
  else
    NODE_COORDINATORS+=(false)
  fi
done

# ------------------------------------------------------------------------------
# Utility & scenario-related functions
# ------------------------------------------------------------------------------

# Function to log messages with timestamps
log() {
  local message="$1"
  echo "$(date '+%Y-%m-%d %H:%M:%S') - $message" | tee -a "$CENTRAL_LOG"
}

# Function to get SharedVar via /sharedVar
get_shared_var() {
  local rest_port=$1
  local node_id=$2
  # Fetch SharedVar using curl and parse JSON with jq
  shared_var=$(curl -s http://127.0.0.1:"$rest_port"/sharedVar | jq '.sharedVar')
  if [[ $? -ne 0 ]]; then
    log "Error fetching SharedVar from $node_id on port $rest_port"
  else
    log "Node $node_id SharedVar: $shared_var"
  fi
}

# Function to start a node
start_node() {
  local index=$1
  local node_id=${NODE_IDS[$index]}
  local ip=${NODE_IPS[$index]}
  local port=${NODE_PORTS[$index]}
  local is_coord=${NODE_COORDINATORS[$index]}
  local rest_port=${NODE_REST_PORTS[$index]}
  local log_file="${LOG_DIR}/${node_id}.log"

  log "Starting $node_id on $ip:$port (Coordinator: $is_coord), REST API on port $rest_port"

  if [ "$is_coord" = true ]; then
    # Coordinator node => 5 arguments
    ./ringnode "$node_id" "$ip" "$port" "true" "$rest_port" >"$log_file" 2>&1 &
  else
    # Non-coordinator => pass coordinator address (the IP & gRPC port of node1, typically index=0)
    local coord_ip=${NODE_IPS[0]}
    local coord_port=${NODE_PORTS[0]}
    ./ringnode "$node_id" "$ip" "$port" "false" "$rest_port" \
      "${coord_ip}:${coord_port}" >"$log_file" 2>&1 &
  fi

  # Capture PID
  echo $! >"${LOG_DIR}/${node_id}.pid"
  sleep 1
}

# Function to stop a node's gRPC server via /kill
kill_node() {
  local rest_port=$1
  local node_id=$2
  log "Sending /kill to REST API on port $rest_port for $node_id"
  curl -s -X POST http://127.0.0.1:"$rest_port"/kill
  log "Node $node_id has been killed."
}

# Function to revive a node's gRPC server via /revive
revive_node() {
  local rest_port=$1
  local node_id=$2
  log "Sending /revive to REST API on port $rest_port for $node_id"
  curl -s -X POST http://127.0.0.1:"$rest_port"/revive
  log "Node $node_id has been revived."
}

# Function to join a node to the ring via /join
join_ring() {
  local coordinator_rest_port=$1
  local node_id=$2
  local ip=$3
  local port=$4

  log "Node $node_id joining the ring via REST API on port $coordinator_rest_port"
  curl -s -X POST -H "Content-Type: application/json" \
    -d "{\"node_id\":\"$node_id\",\"ip\":\"$ip\",\"port\":$port}" \
    http://127.0.0.1:"$coordinator_rest_port"/join
  log "Node $node_id has joined the ring."
}

# Function to enter critical section via /enterCS
enter_cs() {
  local rest_port=$1
  local node_id=$2
  log "Node $node_id entering critical section via REST API on port $rest_port"
  response=$(curl -s -X POST http://127.0.0.1:"$rest_port"/enterCS)
  log "Node $node_id has entered the critical section. Response: $response"
}

# Function to leave critical section via /leaveCS
leave_cs() {
  local rest_port=$1
  local node_id=$2
  log "Node $node_id leaving critical section via REST API on port $rest_port"
  response=$(curl -s -X POST http://127.0.0.1:"$rest_port"/leaveCS)
  log "Node $node_id has left the critical section. Response: $response"
}

# Function to start snapshot via /startSnapshot
start_snapshot() {
  local rest_port=$1
  local node_id=$2
  log "Node $node_id starting snapshot via REST API on port $rest_port"
  response=$(curl -s -X POST http://127.0.0.1:"$rest_port"/startSnapshot)
  log "Snapshot initiated by node $node_id. Response: $response"
}

# Function to clean up background processes
cleanup() {
  log "Cleaning up background node processes..."
  for pid_file in "$LOG_DIR"/*.pid; do
    if [ -f "$pid_file" ]; then
      pid=$(cat "$pid_file")
      node_id="${pid_file%.pid}"
      node_id="${node_id##*/}" # Remove directory path
      log "Killing process $pid (from $pid_file - $node_id)"
      kill "$pid" 2>/dev/null || true
      rm "$pid_file"
    fi
  done
  log "Cleanup complete."
}

# Trap EXIT and other signals to perform cleanup
trap cleanup EXIT INT TERM

# ------------------------------------------------------------------------------
# Scenario Implementations
# ------------------------------------------------------------------------------

# SCENARIO 1: Basic Operation Sequence
execute_scenario_basic() {
  log "Executing Basic Operation Sequence Scenario"

  # Start all nodes
  log "Starting all nodes..."
  for i in "${!NODE_IDS[@]}"; do
    start_node "$i"
  done

  # Wait for nodes to start
  log "Waiting for nodes to initialize..."
  sleep 5

  # Coordinator's REST port
  COORD_REST_PORT=${NODE_REST_PORTS[0]}

  # Have all non-coordinator nodes join the ring via coordinator's REST API
  log "Joining all non-coordinator nodes to the ring..."
  for i in "${!NODE_IDS[@]}"; do
    if [ "$i" -ne 0 ]; then
      join_ring "$COORD_REST_PORT" "${NODE_IDS[$i]}" "${NODE_IPS[$i]}" "${NODE_PORTS[$i]}"
      sleep 1
    fi
  done

  # Wait for joins to complete
  sleep 2

  # Critical Section Operations
  log "Entering critical sections..."
  enter_cs "${NODE_REST_PORTS[0]}" "${NODE_IDS[0]}"
  enter_cs "${NODE_REST_PORTS[1]}" "${NODE_IDS[1]}"
  enter_cs "${NODE_REST_PORTS[2]}" "${NODE_IDS[2]}"

  sleep 2

  log "Leaving critical sections..."
  leave_cs "${NODE_REST_PORTS[0]}" "${NODE_IDS[0]}"
  leave_cs "${NODE_REST_PORTS[1]}" "${NODE_IDS[1]}"
  leave_cs "${NODE_REST_PORTS[2]}" "${NODE_IDS[2]}"

  # Snapshot Operation
  log "Initiating snapshot..."
  start_snapshot "${COORD_REST_PORT}" "${NODE_IDS[0]}"

  # Wait for snapshot to complete
  sleep 5

  # Read log files
  log "Aggregating log files:"
  for node_id in "${NODE_IDS[@]}"; do
    log "$node_id.log:"
    tail -n 10 "${LOG_DIR}/${node_id}.log" | while read -r line; do
      echo "$(date '+%Y-%m-%d %H:%M:%S') - $node_id: $line" | tee -a "$CENTRAL_LOG"
    done
    echo ""
  done

  log "Basic Operation Sequence Scenario Completed"
}

# SCENARIO 2: Fault Tolerance
execute_scenario_fault_tolerance() {
  log "Executing Fault Tolerance Testing Scenario"

  # Start all nodes
  log "Starting all nodes..."
  for i in "${!NODE_IDS[@]}"; do
    start_node "$i"
  done

  # Wait for nodes to start
  log "Waiting for nodes to initialize..."
  sleep 5

  # Coordinator's REST port
  COORD_REST_PORT=${NODE_REST_PORTS[0]}

  # Have all non-coordinator nodes join the ring
  log "Joining all non-coordinator nodes to the ring..."
  for i in "${!NODE_IDS[@]}"; do
    if [ "$i" -ne 0 ]; then
      join_ring "$COORD_REST_PORT" "${NODE_IDS[$i]}" "${NODE_IPS[$i]}" "${NODE_PORTS[$i]}"
      sleep 1
    fi
  done

  # Wait for joins
  sleep 2

  # Enter critical sections
  log "Entering critical sections..."
  enter_cs "${NODE_REST_PORTS[0]}" "${NODE_IDS[0]}"
  enter_cs "${NODE_REST_PORTS[1]}" "${NODE_IDS[1]}"
  enter_cs "${NODE_REST_PORTS[2]}" "${NODE_IDS[2]}"

  # Simulate failure of Node 3
  log "Simulating failure of Node 3..."
  kill_node "${NODE_REST_PORTS[2]}" "${NODE_IDS[2]}"

  # Other nodes attempt to enter CS
  log "Other nodes attempting to enter critical sections..."
  enter_cs "${NODE_REST_PORTS[3]}" "${NODE_IDS[3]}"
  enter_cs "${NODE_REST_PORTS[4]}" "${NODE_IDS[4]}"

  # Revive Node 3
  log "Reviving Node 3 and rejoining the ring..."
  revive_node "${NODE_REST_PORTS[2]}" "${NODE_IDS[2]}"
  join_ring "$COORD_REST_PORT" "${NODE_IDS[2]}" "${NODE_IPS[2]}" "${NODE_PORTS[2]}"
  sleep 1

  # Snapshot
  log "Initiating snapshot after recovery..."
  start_snapshot "$COORD_REST_PORT" "${NODE_IDS[0]}"

  # Wait for snapshot to complete
  sleep 5

  # Read logs
  log "Aggregating log files:"
  for node_id in "${NODE_IDS[@]}"; do
    log "$node_id.log:"
    tail -n 10 "${LOG_DIR}/${node_id}.log" | while read -r line; do
      echo "$(date '+%Y-%m-%d %H:%M:%S') - $node_id: $line" | tee -a "$CENTRAL_LOG"
    done
    echo ""
  done

  log "Fault Tolerance Testing Scenario Completed"
}

# SCENARIO 3: Dynamic Membership Changes
execute_scenario_dynamic_membership() {
  log "Executing Dynamic Membership Changes Scenario"

  # We only want 3 nodes initially, then dynamically add nodes 4 and 5
  local initial_nodes=3

  # Ignore whatever the user passed via -n in this scenario:
  # we force ourselves to start only 3 initially
  NUM_NODES=$initial_nodes

  # Re-initialize the arrays for the *initial* 3 nodes
  NODE_IDS=()
  NODE_IPS=()
  NODE_PORTS=()
  NODE_REST_PORTS=()
  NODE_COORDINATORS=()

  for ((i = 1; i <= initial_nodes; i++)); do
    local node_id="node$i"
    local node_ip="127.0.0.1"
    local node_port=$((BASE_GRPC_PORT + i))
    local node_rest_port=$((BASE_REST_PORT + i))

    NODE_IDS+=("$node_id")
    NODE_IPS+=("$node_ip")
    NODE_PORTS+=("$node_port")
    NODE_REST_PORTS+=("$node_rest_port")

    if [ "$i" -eq 1 ]; then
      NODE_COORDINATORS+=(true)
    else
      NODE_COORDINATORS+=(false)
    fi
  done

  # Start ONLY these first 3 nodes
  log "Starting initial nodes..."
  for i in "${!NODE_IDS[@]}"; do
    start_node "$i"
  done

  # Wait for them to come up
  log "Waiting for initial nodes to initialize..."
  sleep 5

  # The coordinator is node1 => index=0
  COORD_REST_PORT=${NODE_REST_PORTS[0]}

  # Now join node2..node3 to the ring via node1's REST
  log "Joining initial non-coordinator nodes to the ring..."
  for i in "${!NODE_IDS[@]}"; do
    if [ "$i" -ne 0 ]; then
      join_ring "$COORD_REST_PORT" "${NODE_IDS[$i]}" "${NODE_IPS[$i]}" "${NODE_PORTS[$i]}"
      sleep 1
    fi
  done

  sleep 2

  # Some critical section operations
  log "Performing critical section operations..."
  enter_cs "${NODE_REST_PORTS[0]}" "${NODE_IDS[0]}"
  leave_cs "${NODE_REST_PORTS[0]}" "${NODE_IDS[0]}"
  enter_cs "${NODE_REST_PORTS[1]}" "${NODE_IDS[1]}"
  leave_cs "${NODE_REST_PORTS[1]}" "${NODE_IDS[1]}"
  enter_cs "${NODE_REST_PORTS[2]}" "${NODE_IDS[2]}"
  leave_cs "${NODE_REST_PORTS[2]}" "${NODE_IDS[2]}"

  # Now "dynamically" add nodes 4 and 5
  log "Adding new nodes dynamically..."
  local new_nodes=(4 5)
  for node_num in "${new_nodes[@]}"; do
    local node_id="node$node_num"
    local node_ip="127.0.0.1"
    local node_port=$((BASE_GRPC_PORT + node_num))
    local node_rest_port=$((BASE_REST_PORT + node_num))

    # Append these new nodes to the arrays
    NODE_IDS+=("$node_id")
    NODE_IPS+=("$node_ip")
    NODE_PORTS+=("$node_port")
    NODE_REST_PORTS+=("$node_rest_port")
    NODE_COORDINATORS+=(false)

    # Index in the arrays is (node_num - 1)
    local index=$((node_num - 1))
    start_node "$index"
    sleep 2

    # Ask the coordinator to add them to the ring
    join_ring "$COORD_REST_PORT" "$node_id" "$node_ip" "$node_port"
    sleep 1
  done

  # Let them stabilize
  sleep 3

  # Remove a node gracefully (e.g., Node 2)
  log "Removing Node 2 gracefully..."
  # First, leave CS (if we want to guarantee a clean state) – though here it’s just a no-op
  leave_cs "${NODE_REST_PORTS[1]}" "${NODE_IDS[1]}"
  # Then kill node2
  kill_node "${NODE_REST_PORTS[1]}" "${NODE_IDS[1]}"
  sleep 2

  # Initiate snapshot
  log "Initiating snapshot after membership changes..."
  start_snapshot "$COORD_REST_PORT" "${NODE_IDS[0]}"

  # Wait for snapshot to complete
  sleep 5

  # Read log files
  log "Aggregating log files:"
  for node_id in "${NODE_IDS[@]}"; do
    log "$node_id.log:"
    tail -n 10 "${LOG_DIR}/${node_id}.log" | while read -r line; do
      echo "$(date '+%Y-%m-%d %H:%M:%S') - $node_id: $line" | tee -a "$CENTRAL_LOG"
    done
    echo ""
  done

  log "Dynamic Membership Changes Scenario Completed"
}

# SCENARIO 4: Coordinator Forwarding Test
execute_scenario_coordinator_forwarding() {
  log "Executing Coordinator Forwarding Test Scenario"

  # Start all nodes
  log "Starting $NUM_NODES node(s)..."
  for i in "${!NODE_IDS[@]}"; do
    start_node "$i"
  done

  # Wait for nodes to start
  log "Waiting for nodes to initialize..."
  sleep 5

  # The coordinator is node1 => index=0
  local COORD_REST_PORT=${NODE_REST_PORTS[0]}

  # Join all non-coordinator nodes
  log "Joining all non-coordinator nodes to the ring..."
  for i in "${!NODE_IDS[@]}"; do
    if [ "$i" -ne 0 ]; then
      join_ring "$COORD_REST_PORT" "${NODE_IDS[$i]}" "${NODE_IPS[$i]}" "${NODE_PORTS[$i]}"
      sleep 1
    fi
  done

  sleep 2

  # We'll pick one non-coordinator node, e.g. node2 => index=1
  local NON_COORD_INDEX=1
  local NON_COORD_REST_PORT=${NODE_REST_PORTS[$NON_COORD_INDEX]}
  local NON_COORD_ID=${NODE_IDS[$NON_COORD_INDEX]}

  log "Requesting /enterCS from $NON_COORD_ID (non-coordinator). Should be forwarded to coordinator."
  enter_cs "$NON_COORD_REST_PORT" "$NON_COORD_ID"

  get_shared_var "${NODE_REST_PORTS[0]}" "${NODE_IDS[0]}"
  get_shared_var "$NON_COORD_REST_PORT" "$NON_COORD_ID"

  sleep 1

  log "Now the coordinator itself enters CS."
  enter_cs "$COORD_REST_PORT" "${NODE_IDS[0]}"

  get_shared_var "${NODE_REST_PORTS[0]}" "${NODE_IDS[0]}"
  get_shared_var "$NON_COORD_REST_PORT" "$NON_COORD_ID"

  sleep 2

  log "Leaving CS from the non-coordinator node."
  leave_cs "$NON_COORD_REST_PORT" "$NON_COORD_ID"
  get_shared_var "${NODE_REST_PORTS[0]}" "${NODE_IDS[0]}"
  get_shared_var "$NON_COORD_REST_PORT" "$NON_COORD_ID"

  log "Leaving CS from the coordinator node."
  leave_cs "$COORD_REST_PORT" "${NODE_IDS[0]}"
  get_shared_var "${NODE_REST_PORTS[0]}" "${NODE_IDS[0]}"
  get_shared_var "$NON_COORD_REST_PORT" "$NON_COORD_ID"

  # Optional: Start a snapshot
  log "Initiating snapshot from coordinator..."
  start_snapshot "$COORD_REST_PORT" "${NODE_IDS[0]}"

  sleep 5

  # Read and aggregate logs
  log "Aggregating log files for final inspection..."
  for node_id in "${NODE_IDS[@]}"; do
    log "$node_id.log:"
    tail -n 10 "${LOG_DIR}/${node_id}.log" | while read -r line; do
      echo "$(date '+%Y-%m-%d %H:%M:%S') - $node_id: $line" | tee -a "$CENTRAL_LOG"
    done
    echo ""
  done

  # Fetch and log SharedVar after snapshot
  log "Fetching SharedVar values after snapshot:"
  for node_id in "${NODE_IDS[@]}"; do
    # find index
    for idx in "${!NODE_IDS[@]}"; do
      if [ "${NODE_IDS[$idx]}" == "$node_id" ]; then
        get_shared_var "${NODE_REST_PORTS[$idx]}" "$node_id"
        break
      fi
    done
  done

  log "Coordinator Forwarding Test Scenario Completed"
}

# ------------------------------------------------------------------------------
# Dispatcher
# ------------------------------------------------------------------------------

execute_selected_scenario() {
  case "$SCENARIO" in
  "basic")
    execute_scenario_basic
    ;;
  "fault_tolerance")
    execute_scenario_fault_tolerance
    ;;
  "dynamic_membership")
    execute_scenario_dynamic_membership
    ;;
  "coordinator_forwarding")
    execute_scenario_coordinator_forwarding
    ;;
  *)
    log "Unknown scenario: $SCENARIO"
    usage
    ;;
  esac
}

# Run the chosen scenario
execute_selected_scenario

# Final log message
log "Script completed successfully."
