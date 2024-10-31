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
	Namespace   map[string]ParamSchema `json:"namespace"`
	Config      map[string]ParamSchema `json:"config"`
}

type ParamSchema struct {
	Type        string      `json:"type"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default"`
	Description string      `json:"description"`
}

type OperatorParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     string `json:"default"`
	Description string `json:"description"`
}

type OperatorPayload struct {
	Operation string                 `json:"operation"`
	Namespace map[string]string      `json:"namespace,omitempty"`
	Config    map[string]interface{} `json:"config,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`
}

var (
	config  Config
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
	rootCmd.AddCommand(newCmd("clusters", "Get avaliable clusters", getClusterList))

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

	// Add parameter flags
	triggerCmd.Flags().StringToStringP("params", "p", nil, "Operation parameters (key=value)")
	triggerCmd.Flags().StringToStringP("config", "c", nil, "Config parameters (key=value)")
	triggerCmd.Flags().StringToStringP("namespace", "n", nil, "Namespace parameters (key=value)")

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

func fetchFromAPI(cluster *ClusterConfig, endpoint string) (*APIResponse, error) {
	var nodeAddr string
	for _, addr := range cluster.Nodes {
		nodeAddr = addr
		break
	}
	client := &http.Client{Timeout: time.Duration(config.Timeout) * time.Second}
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

func triggerOperator(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: gocluster operator trigger <operator_name> <operation> [-p key=value] [-n key=value] [-c key=value]")
		return
	}

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
	namespace, _ := cmd.Flags().GetStringToString("namespace")
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

	validatedNamespace, err := validateAndConvertParams(namespace, opSchema.Namespace)
	if err != nil {
		fmt.Printf("Namespace validation error: %v\n", err)
		return
	}

	validatedConfig, err := validateAndConvertParams(config, opSchema.Config)
	if err != nil {
		fmt.Printf("Config validation error: %v\n", err)
		return
	}

	payload := OperatorPayload{
		Operation: operationName,
		Params:    validatedParams,
		Namespace: stringMapFromInterface(validatedNamespace),
		Config:    validatedConfig,
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
	}
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

func validateAndConvertParams(params map[string]string, schema map[string]ParamSchema) (map[string]interface{}, error) {
	result := make(map[string]interface{})

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

func stringMapFromInterface(m map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[k] = fmt.Sprint(v)
	}
	return result
}
