<div align="center">

# 🌟 GoCluster-CLI

[![MIT License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

</div>

GoCluster is a command-line interface (CLI) tool for managing multi-goclusters at the tips of your hands, Please visit the [GoCluster GitHub repository](https://github.com/Prajwalprakash3722/gocluster) to understand what is GoCluster and how to use it.

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
