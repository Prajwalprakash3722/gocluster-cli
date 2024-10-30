package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type MultiClusterConfig struct {
	Clusters map[string]ClusterConfig `mapstructure:"clusters"`
	CLI      CLIConfig                `mapstructure:"cli"`
}

type CLIConfig struct {
	DefaultCluster string `mapstructure:"default_cluster"`
	Timeout        int    `mapstructure:"timeout"`
	Retries        int    `mapstructure:"retries"`
}

type ClusterConfig struct {
	Name  string            `mapstructure:"name"`
	Nodes map[string]string `mapstructure:"nodes"`
	Port  int               `mapstructure:"port"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   string      `json:"error"`
}

var (
	config  MultiClusterConfig
	rootCmd = &cobra.Command{Use: "gocluster"}
)

func main() {
	cobra.OnInitialize(initConfig)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&config.CLI.DefaultCluster, "cluster", "c", "", "Specify the cluster name")
	rootCmd.AddCommand(newCmd("health", "Check cluster health", checkHealth))
	rootCmd.AddCommand(newCmd("nodes", "List all nodes in the cluster", listNodes))
	rootCmd.AddCommand(newCmd("leader", "Get current cluster leader", getLeader))
}

func newCmd(use, short string, run func(cmd *cobra.Command, args []string)) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Run: run}
}

func initConfig() {
	viper.SetConfigType("yaml")
	viper.SetConfigName(".gocluster")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Unable to read config:", err)
		os.Exit(1)
	}
	if err := viper.Unmarshal(&config); err != nil {
		fmt.Println("Unable to decode config:", err)
		os.Exit(1)
	}
}

func getActiveCluster(args []string) (*ClusterConfig, error) {
	if len(args) > 0 {
		config.CLI.DefaultCluster = args[0]
	}
	cluster, exists := config.Clusters[config.CLI.DefaultCluster]
	if !exists {
		return nil, fmt.Errorf("cluster %s not found in configuration", config.CLI.DefaultCluster)
	}
	return &cluster, nil
}

func fetchFromAPI(cluster *ClusterConfig, endpoint string) (*APIResponse, error) {
	var nodeAddr string
	for _, addr := range cluster.Nodes {
		nodeAddr = addr
		break
	}
	client := &http.Client{Timeout: time.Duration(config.CLI.Timeout) * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/api/%s", nodeAddr, endpoint))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}
	return &apiResp, nil
}

func checkHealth(cmd *cobra.Command, args []string) {
	cluster, err := getActiveCluster(args)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	resp, err := fetchFromAPI(cluster, "health")
	if err != nil {
		fmt.Println("Error checking health:", err)
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Node", "Status", "Last Seen"})
	for node, addr := range cluster.Nodes {
		status := "Healthy"
		if resp.Success == false {
			status = "Unhealthy"
		}
		table.Append([]string{node, status, addr})
	}
	table.Render()
}

func listNodes(cmd *cobra.Command, args []string) {
	cluster, err := getActiveCluster(args)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	resp, err := fetchFromAPI(cluster, "nodes")
	if err != nil {
		fmt.Println("Error fetching nodes:", err)
		return
	}

	nodes, ok := resp.Data.([]interface{})
	if !ok {
		fmt.Println("Invalid response format for nodes")
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Node ID", "Address", "Last Seen", "State"})
	for _, nodeData := range nodes {
		nodeMap, ok := nodeData.(map[string]interface{})
		if !ok {
			fmt.Println("Invalid node data format")
			continue
		}

		id, _ := nodeMap["id"].(string)
		address, _ := nodeMap["address"].(string)
		lastSeen, _ := nodeMap["last_seen"].(string)
		state, _ := nodeMap["state"].(string)

		table.Append([]string{id, address, lastSeen, state})
	}
	table.Render()
}

func getLeader(cmd *cobra.Command, args []string) {
	cluster, err := getActiveCluster(args)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	resp, err := fetchFromAPI(cluster, "leader")
	if err != nil {
		fmt.Println("Error fetching leader:", err)
		return
	}

	leader, ok := resp.Data.(map[string]interface{})
	if !ok {
		fmt.Println("Invalid response format for leader")
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Leader ID", "Address"})
	table.Append([]string{leader["id"].(string), leader["address"].(string)})
	table.Render()
}
