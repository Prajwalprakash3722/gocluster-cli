<div align="center">

# 🌟 GoCluster-CLI

[![MIT License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

</div>

GoCluster is a command-line interface (CLI) tool for interacting with multi-goclusters at the tips of your hands, Please visit the [GoCluster GitHub repository](https://github.com/Prajwalprakash3722/gocluster) to understand what is GoCluster and how to use it.

## Installation & Usage

### Prerequisites

```bash
- Go 1.21+
- Linux/Unix environment, if you have windows, please do a favor to yourself and throw it away:)
```
### Clone the repository:

```bash
git clone https://github.com/Prajwalprakash3722/gocluster-cli.git
cd gocluster-cli
```
### Build the project:

```bash
make {build|linux} # build for macos
```
### Run the CLI:

```bash
./gocluster --help

Usage:
  gocluster [command]

Available Commands:
  clusters    Get available clusters
  completion  Generate the autocompletion script for the specified shell
  config      Manage cluster configuration
  health      Check cluster health
  help        Help about any command
  leader      Get current cluster leader
  logs        View cluster logs
  metrics     View cluster metrics
  nodes       List all nodes in the cluster
  operator    Operator commands
  use         Select a cluster to use
  which       Show currently selected cluster

Flags:
  -h, --help            help for gocluster
      --nodes strings   Specific nodes to run operation on (comma-separated)
      --parallel        Run operations in parallel (default true)

Use "gocluster [command] --help" for more information about a command.
```

## Configuration

GoCluster-CLI requires a YAML configuration file named .gocluster.yaml in the $HOME directory or same directory as the executable. Below is an example configuration:

```yaml
cli:
  default_cluster: "stg-nodes" # Default cluster for operations
  timeout: 10                  # Request timeout in seconds
  retries: 3                   # Number of retries for failed requests

clusters:
  stg-nodes:
    nodes:
      node001: "node001.example.com:8080"
      node002: "node002.example.com:8080"
      node003: "node003.example.com:8080"
      node004: "node004.example.com:8080"
    name: "stg-nodes"
    port: 7946
```
## Usage Examples

### Select Cluster

```bash
$ gocluster use stg-nodes
```

### Show Currently Selected Cluster

```bash
$ gocluster which
Currently selected cluster: stg-nodes
```

### List All Clusters

```bash
$ gocluster clusters
+--------------------+
| AVALIABLE CLUSTERS |
+--------------------+
| local              |
| stg-nodes          |
+--------------------+
```

### List Cluster Nodes

```bash
$ gocluster nodes stg-nodes
+-----------+--------------------------+-------------------------------------+----------+
|  NODE ID  |          ADDRESS         |              LAST SEEN              |  STATE   |
+-----------+--------------------------+-------------------------------------+----------+
| node003   | node003.example.com:8080 | 2024-10-30T17:15:30.487221912+05:30| follower  |
| node004   | node004.example.com:8080 | 2024-10-30T17:15:30.491098466+05:30| follower  |
| node002   | node002.example.com:8080 | 2024-10-30T17:15:30.482072842+05:30| follower  |
+-----------+--------------------------+-------------------------------------+----------+
```

### Check Cluster Health

```bash
$ gocluster health stg-nodes
+-----------+-----------+---------------------------+
|   NODE    |  STATUS   |         ADDRESS           |
+-----------+-----------+---------------------------+
| node004   | Healthy   | node004.example.com:8080  |
| node003   | Healthy   | node003.example.com:8080  |
| node002   | Healthy   | node002.example.com:8080  |
| node001   | Healthy   | node001.example.com:8080  |
+-----------+-----------+---------------------------+
```

### Get Cluster Leader

```bash
$ gocluster leader stg-nodes
+-----------+--------------------------+
| LEADER ID |         ADDRESS          |
+-----------+--------------------------+
| node001   | node001.example.com:8080 |
+-----------+--------------------------+
```

### List Enabled Operators (Experimental)

```bash
$ gocluster operator list                                                                                                                     on  main [! ] via 🐹 v1.21.13 󰘧

Available Operators
Use 'gocluster operator show <name>' for detailed information

+-----------+---------+-----------+-------------------------------------------------+
|   NAME    | VERSION |  AUTHOR   |                   DESCRIPTION                   |
+-----------+---------+-----------+-------------------------------------------------+
| aerospike |  1.0.0  | prajwal.p | Aerospike configuration and management operator |
+-----------+---------+-----------+-------------------------------------------------+
```

### Show Operator Details (Experimental)

```bash
$ gocluster operator show aerospike

Operator: aerospike
Version:     1.0.0
Description: Aerospike configuration and management operator

Available Operations
-------------------

add_namespace
Add new namespace to Aerospike configuration

Parameters:
+-----------------------+--------+----------+---------+--------------------------------+
|         NAME          |  TYPE  | REQUIRED | DEFAULT |          DESCRIPTION           |
+-----------------------+--------+----------+---------+--------------------------------+
| high_water_memory_pct | int    |  false   | 70      | High water memory percentage   |
| name                  | string |   true   | nil     | Namespace name                 |
| replication_factor    | int    |  false   | 2       | Replication factor             |
| storage_engine        | string |  false   | device  | Storage engine type            |
|                       |        |          |         | (memory|device)                |
| data_in_memory        | bool   |  false   | true    | Keep data in memory            |
| default_ttl           | int    |  false   | 0       | Default TTL in seconds (0 =    |
|                       |        |          |         | never expire)                  |
| high_water_disk_pct   | int    |  false   | 70      | High water disk percentage     |
| memory_size           | string |  false   | 1G      | Memory size for namespace      |
| stop_writes_pct       | int    |  false   | 90      | Stop writes percentage         |
|                       |        |          |         | threshold                      |
+-----------------------+--------+----------+---------+--------------------------------+
```

### Trigger a Operation for Operator (Experimental)

```bash
$ gocluster operator trigger aerospike add_namespace -p name=testing-for-github -c config_path=/etc/aerospike/aerospike.conf -p high_water_disk_pct=40gocluster operator trigger aerospike add_namespace -p name=testing-for-github -c config_path=/etc/aerospike/aerospike.conf -p high_water_disk_pct=40

Operation triggered successfully
```

### Get Cluster Logs (Experimental)

```bash
$ gocluster logs --lines 5 --level DEBUG

[2024-11-10 16:48:10] [INFO] Broadcasting heartbeat
[2024-11-10 16:48:12] [INFO] Broadcasting heartbeat
[2024-11-10 16:48:14] [INFO] Broadcasting heartbeat
[2024-11-10 16:48:16] [INFO] Broadcasting heartbeat
[2024-11-10 16:48:18] [INFO] Broadcasting heartbeat
```

If you pass DEBUG, then the logs which are DEBUG and above will be shown, similarly for INFO, ERROR.  Default is INFO.
Default lines are 100.

### Get Specific Node Logs (Experimental)

```bash
$ gocluster logs --lines 5 --level DEBUG --node node001

[2024-11-10 16:48:10] [INFO] Broadcasting heartbeat
[2024-11-10 16:48:12] [INFO] Broadcasting heartbeat
[2024-11-10 16:48:14] [INFO] Broadcasting heartbeat
[2024-11-10 16:48:16] [INFO] Broadcasting heartbeat
[2024-11-10 16:48:18] [INFO] Broadcasting heartbeat
```

## Add Completion to your shell
```bash
$ gocluster completion zsh > ~/.zsh/completion/_gocluster
```
