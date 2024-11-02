package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Config struct {
	Clusters        map[string]ClusterConfig `mapstructure:"clusters"`
	SelectedCluster string                   `mapstructure:"selected_cluster"`
	Timeout         int                      `mapstructure:"timeout"`
	Retries         int                      `mapstructure:"retries"`
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

type OperatorSchema struct {
	Name        string                     `json:"name"`
	Version     string                     `json:"version"`
	Description string                     `json:"description"`
	Operations  map[string]OperationSchema `json:"operations"`
}

type OperationSchema struct {
	Description string                 `json:"description"`
	Parameters  map[string]ParamSchema `json:"parameters"`
	Config      map[string]ParamSchema `json:"config"`
}

type ParamSchema struct {
	Type        string      `json:"type"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default"`
	Description string      `json:"description"`
}

type OperatorPayload struct {
	Operation   string                 `json:"operation"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Params      map[string]interface{} `json:"params,omitempty"`
	Parallel    bool                   `json:"parallel"`
	TargetNodes []string               `json:"target_nodes,omitempty"`
}

// Global flags
var (
	parallel    bool
	targetNodes []string
	logNode     string
	logLines    int
	followLogs  bool
	config      Config
	rootCmd     = &cobra.Command{Use: "gocluster"}
)

func initConfig() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error finding home directory:", err)
		os.Exit(1)
	}

	viper.SetConfigType("yaml")
	viper.SetConfigName(".gocluster")
	viper.AddConfigPath(".")
	viper.AddConfigPath(home)

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Unable to read config:", err)
		os.Exit(1)
	}

	if err := viper.Unmarshal(&config); err != nil {
		fmt.Println("Unable to decode config:", err)
		os.Exit(1)
	}
}

func main() {
	cobra.OnInitialize(initConfig)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVar(&parallel, "parallel", true, "Run operations in parallel")
	rootCmd.PersistentFlags().StringSliceVar(&targetNodes, "nodes", []string{}, "Specific nodes to run operation on (comma-separated)")

	// Cluster management commands
	rootCmd.AddCommand(
		&cobra.Command{
			Use:   "use [cluster_name]",
			Short: "Select a cluster to use",
			Args:  cobra.ExactArgs(1),
			Run:   useCluster,
		},
		&cobra.Command{
			Use:   "which",
			Short: "Show currently selected cluster",
			Run:   showSelectedCluster,
		},
	)

	// Basic commands
	rootCmd.AddCommand(newCmd("health", "Check cluster health", checkHealth))
	rootCmd.AddCommand(newCmd("nodes", "List all nodes in the cluster", listNodes))
	rootCmd.AddCommand(newCmd("leader", "Get current cluster leader", getLeader))
	rootCmd.AddCommand(newCmd("clusters", "Get available clusters", getClusterList))

	// Logs command
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "View cluster logs",
		Run:   viewLogs,
	}
	logsCmd.Flags().StringVarP(&logNode, "node", "n", "", "Node to fetch logs from (defaults to leader)")
	logsCmd.Flags().IntVarP(&logLines, "lines", "l", 100, "Number of log lines to fetch")
	logsCmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Stream logs in real-time")
	rootCmd.AddCommand(logsCmd)

	// Metrics command
	metricsCmd := &cobra.Command{
		Use:   "metrics",
		Short: "View cluster metrics",
		Run:   viewMetrics,
	}
	rootCmd.AddCommand(metricsCmd)

	// Config command
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage cluster configuration",
	}
	configCmd.AddCommand(
		&cobra.Command{
			Use:   "view",
			Short: "View current configuration",
			Run:   viewConfig,
		},
		&cobra.Command{
			Use:   "set [key] [value]",
			Short: "Set configuration value",
			Args:  cobra.ExactArgs(2),
			Run:   setConfig,
		},
	)
	rootCmd.AddCommand(configCmd)

	// Operator commands
	operatorCmd := &cobra.Command{
		Use:   "operator",
		Short: "Operator commands",
	}

	triggerCmd := &cobra.Command{
		Use:   "trigger [operator_name] [operation]",
		Short: "Trigger operator operation",
		Args:  cobra.ExactArgs(2),
		Run:   triggerOperator,
	}

	triggerCmd.Flags().StringToStringP("params", "p", nil, "Operation parameters (key=value)")
	triggerCmd.Flags().StringToStringP("config", "c", nil, "Config parameters (key=value)")

	operatorCmd.AddCommand(
		&cobra.Command{
			Use:   "list [operator_name]",
			Short: "List available operators or show detailed info for a specific operator",
			Run:   listOperators,
		},
		&cobra.Command{
			Use:   "show [operator_name]",
			Short: "Show detailed information for a specific operator",
			Args:  cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				cluster, err := getSelectedCluster()
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					return
				}
				showOperatorDetails(cluster, args[0])
			},
		},
		triggerCmd,
	)

	rootCmd.AddCommand(operatorCmd)
}

func newCmd(use, short string, run func(cmd *cobra.Command, args []string)) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Run: run}
}

func useCluster(cmd *cobra.Command, args []string) {
	clusterName := args[0]

	if _, exists := config.Clusters[clusterName]; !exists {
		fmt.Printf("Cluster '%s' not found. Available clusters:\n", clusterName)
		for name := range config.Clusters {
			fmt.Printf("- %s\n", name)
		}
		return
	}

	config.SelectedCluster = clusterName
	viper.Set("selected_cluster", clusterName)

	configPath := viper.ConfigFileUsed()
	if err := viper.WriteConfig(); err != nil {
		if os.IsNotExist(err) {
			dir := filepath.Dir(configPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Printf("Error creating config directory: %v\n", err)
				return
			}
			if err := viper.WriteConfigAs(configPath); err != nil {
				fmt.Printf("Error saving config: %v\n", err)
				return
			}
		} else {
			fmt.Printf("Error saving config: %v\n", err)
			return
		}
	}

	fmt.Printf("Now using cluster: %s\n", clusterName)
}

func getClusterList(cmd *cobra.Command, args []string) {
	// put it in a neat table format
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Avaliable Clusters"})
	for name := range config.Clusters {
		table.Append([]string{name})
	}
	table.Render()
	return
}

func showSelectedCluster(cmd *cobra.Command, args []string) {
	if config.SelectedCluster == "" {
		fmt.Println("No cluster selected. Use 'gocluster use <cluster_name>' to select a cluster.")
		return
	}
	fmt.Printf("Currently selected cluster: %s\n", config.SelectedCluster)
}

func getSelectedCluster() (*ClusterConfig, error) {
	if config.SelectedCluster == "" {
		return nil, fmt.Errorf("no cluster selected. Use 'gocluster use <cluster_name>' to select a cluster")
	}
	cluster, exists := config.Clusters[config.SelectedCluster]
	if !exists {
		return nil, fmt.Errorf("selected cluster %s not found in configuration", config.SelectedCluster)
	}
	return &cluster, nil
}

// New command implementations
func viewLogs(cmd *cobra.Command, args []string) {
	cluster, err := getSelectedCluster()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	targetNode := logNode
	if targetNode == "" {
		// Get leader node if no specific node is specified
		resp, err := fetchFromAPI(cluster, "leader")
		if err != nil {
			fmt.Printf("Error fetching leader: %v\n", err)
			return
		}
		leader, ok := resp.Data.(map[string]interface{})
		if !ok {
			fmt.Println("Invalid response format for leader")
			return
		}
		targetNode = leader["id"].(string)
	}

	endpoint := fmt.Sprintf("logs/%s?lines=%d", targetNode, logLines)
	if followLogs {
		endpoint += "&follow=true"
	}

	resp, err := fetchFromAPI(cluster, endpoint)
	if err != nil {
		fmt.Printf("Error fetching logs: %v\n", err)
		return
	}

	logs, ok := resp.Data.([]interface{})
	if !ok {
		fmt.Println("Invalid response format for logs")
		return
	}

	for _, log := range logs {
		fmt.Println(log)
	}

	if followLogs {
		fmt.Println("Log streaming not implemented yet")
	}
}

func showOperatorDetails(cluster *ClusterConfig, operatorName string) {
	schema, err := fetchOperatorSchema(cluster, operatorName)
	if err != nil {
		fmt.Printf("Error fetching operator details: %v\n", err)
		return
	}

	fmt.Printf("\nOperator: %s\n", schema.Name)
	fmt.Printf("Version:     %s\n", schema.Version)
	fmt.Printf("Description: %s\n\n", schema.Description)

	fmt.Println("Available Operations")
	fmt.Println("-------------------")

	for opName, opSchema := range schema.Operations {
		fmt.Printf("\n%s\n", opName)
		fmt.Printf("%s\n", opSchema.Description)

		// Parameters table
		if len(opSchema.Parameters) > 0 {
			fmt.Println("\nParameters:")
			paramTable := tablewriter.NewWriter(os.Stdout)
			paramTable.SetHeader([]string{"Name", "Type", "Required", "Default", "Description"})
			paramTable.SetColumnAlignment([]int{
				tablewriter.ALIGN_LEFT,
				tablewriter.ALIGN_LEFT,
				tablewriter.ALIGN_CENTER,
				tablewriter.ALIGN_LEFT,
				tablewriter.ALIGN_LEFT,
			})

			for name, param := range opSchema.Parameters {
				defaultVal := "nil"
				if param.Default != nil {
					defaultVal = fmt.Sprintf("%v", param.Default)
				}
				paramTable.Append([]string{
					name,
					param.Type,
					fmt.Sprintf("%v", param.Required),
					defaultVal,
					param.Description,
				})
			}
			paramTable.Render()
		}
	}
	fmt.Println()
}

func checkHealth(cmd *cobra.Command, args []string) {
	cluster, err := getSelectedCluster()
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
	table.SetHeader([]string{"Node", "Status", "Address"})
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
	cluster, err := getSelectedCluster()
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
	table.SetHeader([]string{"Node ID", "Address", "Age", "State"})
	for _, nodeData := range nodes {
		nodeMap, ok := nodeData.(map[string]interface{})
		if !ok {
			fmt.Println("Invalid node data format")
			continue
		}

		id, _ := nodeMap["id"].(string)
		address, _ := nodeMap["address"].(string)
		lastSeen, _ := time.Parse(time.RFC3339Nano, nodeMap["last_seen"].(string))
		age := humanize.Time(lastSeen)
		state, _ := nodeMap["state"].(string)

		table.Append([]string{id, address, age, state})
	}
	table.Render()
}

func getLeader(cmd *cobra.Command, args []string) {
	cluster, err := getSelectedCluster()
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

func listOperators(cmd *cobra.Command, args []string) {
	cluster, err := getSelectedCluster()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Check if we're showing detailed info for a specific operator
	if len(args) > 0 {
		showOperatorDetails(cluster, args[0])
		return
	}

	resp, err := fetchFromAPI(cluster, "operator/list")
	if err != nil {
		fmt.Printf("Error fetching operators: %v\n", err)
		return
	}

	operators, ok := resp.Data.([]interface{})
	if !ok {
		fmt.Println("Invalid response format")
		return
	}

	// Create and configure table for summary view
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Version", "Author", "Description"})
	table.SetAutoWrapText(false)
	table.SetColumnAlignment([]int{
		tablewriter.ALIGN_LEFT,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_LEFT,
		tablewriter.ALIGN_LEFT,
	})

	for _, op := range operators {
		operator := op.(map[string]interface{})
		table.Append([]string{
			operator["name"].(string),
			operator["version"].(string),
			operator["author"].(string),
			operator["description"].(string),
		})
	}

	fmt.Println("\nAvailable Operators")
	fmt.Println("Use 'gocluster operator show <name>' for detailed information")
	fmt.Println()
	table.Render()
}

func viewMetrics(cmd *cobra.Command, args []string) {
	cluster, err := getSelectedCluster()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	resp, err := fetchFromAPI(cluster, "metrics")
	if err != nil {
		fmt.Printf("Error fetching metrics: %v\n", err)
		return
	}

	metrics, ok := resp.Data.(map[string]interface{})
	if !ok {
		fmt.Println("Invalid response format for metrics")
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Metric", "Value"})

	for metric, value := range metrics {
		table.Append([]string{metric, fmt.Sprintf("%v", value)})
	}
	table.Render()
}

func createBackup(cmd *cobra.Command, args []string) {
	cluster, err := getSelectedCluster()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	backupName := args[0]
	resp, err := fetchFromAPI(cluster, fmt.Sprintf("backup/create/%s", backupName))
	if err != nil {
		fmt.Printf("Error creating backup: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("Backup '%s' created successfully\n", backupName)
	} else {
		fmt.Printf("Failed to create backup: %s\n", resp.Error)
	}
}

func listBackups(cmd *cobra.Command, args []string) {
	cluster, err := getSelectedCluster()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	resp, err := fetchFromAPI(cluster, "backup/list")
	if err != nil {
		fmt.Printf("Error listing backups: %v\n", err)
		return
	}

	backups, ok := resp.Data.([]interface{})
	if !ok {
		fmt.Println("Invalid response format for backups")
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Size", "Created At"})

	for _, backup := range backups {
		b := backup.(map[string]interface{})
		table.Append([]string{
			b["name"].(string),
			b["size"].(string),
			b["created_at"].(string),
		})
	}
	table.Render()
}

func restoreBackup(cmd *cobra.Command, args []string) {
	cluster, err := getSelectedCluster()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	backupName := args[0]
	resp, err := fetchFromAPI(cluster, fmt.Sprintf("backup/restore/%s", backupName))
	if err != nil {
		fmt.Printf("Error restoring backup: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("Backup '%s' restored successfully\n", backupName)
	} else {
		fmt.Printf("Failed to restore backup: %s\n", resp.Error)
	}
}

func viewConfig(cmd *cobra.Command, args []string) {
	cluster, err := getSelectedCluster()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	resp, err := fetchFromAPI(cluster, "config")
	if err != nil {
		fmt.Printf("Error fetching config: %v\n", err)
		return
	}

	config, ok := resp.Data.(map[string]interface{})
	if !ok {
		fmt.Println("Invalid response format for config")
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Key", "Value"})

	for key, value := range config {
		table.Append([]string{key, fmt.Sprintf("%v", value)})
	}
	table.Render()
}

func setConfig(cmd *cobra.Command, args []string) {
	cluster, err := getSelectedCluster()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	key := args[0]
	value := args[1]

	payload := map[string]string{
		"key":   key,
		"value": value,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error encoding payload: %v\n", err)
		return
	}

	var nodeAddr string
	for _, addr := range cluster.Nodes {
		nodeAddr = addr
		break
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST",
		fmt.Sprintf("http://%s/api/config/set", nodeAddr),
		bytes.NewBuffer(payloadBytes))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		fmt.Printf("Error decoding response: %v\n", err)
		return
	}

	if apiResp.Success {
		fmt.Printf("Configuration updated successfully\n")
	} else {
		fmt.Printf("Failed to update configuration: %s\n", apiResp.Error)
	}
}

// Modified triggerOperator to use global flags
func triggerOperator(cmd *cobra.Command, args []string) {
	operatorName := args[0]
	operationName := args[1]

	cluster, err := getSelectedCluster()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	schema, err := fetchOperatorSchema(cluster, operatorName)
	if err != nil {
		fmt.Println("Error fetching operator schema:", err)
		return
	}

	opSchema, exists := schema.Operations[operationName]
	if !exists {
		fmt.Printf("Operation '%s' not found for operator '%s'\n", operationName, operatorName)
		fmt.Println("\nAvailable operations:")
		for op := range schema.Operations {
			fmt.Printf("- %s\n", op)
		}
		return
	}

	params, _ := cmd.Flags().GetStringToString("params")
	config, _ := cmd.Flags().GetStringToString("config")

	validatedParams, err := validateAndConvertParams(params, opSchema.Parameters)
	if err != nil {
		fmt.Printf("Parameter validation error: %v\n", err)
		fmt.Println("\nRequired parameters:")
		for name, param := range opSchema.Parameters {
			if param.Required {
				fmt.Printf("- %s (%s): %s\n", name, param.Type, param.Description)
			}
		}
		return
	}

	validatedConfig, err := validateAndConvertParams(config, opSchema.Config)
	if err != nil {
		fmt.Printf("Config validation error: %v\n", err)
		return
	}

	payload := OperatorPayload{
		Operation:   operationName,
		Params:      validatedParams,
		Config:      validatedConfig,
		Parallel:    parallel,
		TargetNodes: targetNodes,
	}

	var nodeAddr string
	for _, addr := range cluster.Nodes {
		nodeAddr = addr
		break
	}

	client := &http.Client{}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Error encoding payload:", err)
		return
	}

	req, err := http.NewRequest("POST",
		fmt.Sprintf("http://%s/api/operator/trigger/%s", nodeAddr, operatorName),
		bytes.NewBuffer(payloadBytes))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		fmt.Println("Error decoding response:", err)
		return
	}

	if !apiResp.Success {
		fmt.Printf("Failed to trigger operation: %s\n", apiResp.Error)
	} else {
		fmt.Println("Operation triggered successfully")
		if responseData, ok := apiResp.Data.(map[string]interface{}); ok {
			if jobID, exists := responseData["job_id"]; exists {
				fmt.Printf("Job ID: %v\n", jobID)
				fmt.Println("Use 'gocluster operator status <job_id>' to check the status")
			}
		}
	}
}

func validateAndConvertParams(params map[string]string, schema map[string]ParamSchema) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Check for required parameters
	for name, paramSchema := range schema {
		if paramSchema.Required {
			if _, exists := params[name]; !exists {
				if paramSchema.Default != nil {
					result[name] = paramSchema.Default
				} else {
					return nil, fmt.Errorf("required parameter '%s' is missing", name)
				}
			}
		}
	}

	// Convert and validate provided parameters
	for name, value := range params {
		paramSchema, exists := schema[name]
		if !exists {
			return nil, fmt.Errorf("unknown parameter '%s'", name)
		}

		converted, err := convertValue(value, paramSchema.Type)
		if err != nil {
			return nil, fmt.Errorf("parameter '%s': %v", name, err)
		}
		result[name] = converted
	}

	return result, nil
}

func convertValue(value string, targetType string) (interface{}, error) {
	switch targetType {
	case "string":
		return value, nil
	case "int":
		return strconv.Atoi(value)
	case "bool":
		return strconv.ParseBool(value)
	case "float":
		return strconv.ParseFloat(value, 64)
	default:
		return nil, fmt.Errorf("unsupported type: %s", targetType)
	}
}

func fetchFromAPI(cluster *ClusterConfig, endpoint string) (*APIResponse, error) {
	var lastErr error
	for _, addr := range cluster.Nodes {
		client := &http.Client{Timeout: time.Duration(config.Timeout) * time.Second}
		resp, err := client.Get(fmt.Sprintf("http://%s/api/%s", addr, endpoint))
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var apiResp APIResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			lastErr = err
			continue
		}
		return &apiResp, nil
	}
	return nil, fmt.Errorf("failed to fetch from any node: %v", lastErr)
}

func fetchOperatorSchema(cluster *ClusterConfig, operatorName string) (*OperatorSchema, error) {
	resp, err := fetchFromAPI(cluster, fmt.Sprintf("operator/schema/%s", operatorName))
	if err != nil {
		return nil, err
	}

	schemaData, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, err
	}

	var schema OperatorSchema
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		return nil, err
	}

	return &schema, nil
}
