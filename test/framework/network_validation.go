package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// ValidateNetworkUp validates that nodes have 2 different external IPs indicating both NICs are up
func (e *ClusterE2ETest) ValidateNetworkUp() {
	e.T.Log("Validating that nodes have 2 different external IPs (both NICs are up)")

	// Get all nodes
	nodes, err := e.getAllNodes()
	if err != nil {
		e.T.Fatalf("Failed to get nodes: %v", err)
	}

	for _, node := range nodes {
		e.T.Logf("Validating network interfaces for node: %s", node.Name)

		// Extract external IPs from node status
		externalIPs := e.getExternalIPsFromNode(node)

		if len(externalIPs) < 2 {
			e.T.Fatalf("Node %s does not have 2 external IPs. Found %d IPs: %v",
				node.Name, len(externalIPs), externalIPs)
		}

		// Validate that the IPs are different
		if !e.areIPsDifferent(externalIPs) {
			e.T.Fatalf("Node %s has duplicate external IPs: %v", node.Name, externalIPs)
		}

		e.T.Logf("Node %s has %d different external IPs: %v ✓",
			node.Name, len(externalIPs), externalIPs)
	}

	e.T.Log("Network validation completed successfully - all nodes have multiple external IPs")
}

// ValidateNetworkUpWithJSONPath validates network using JSONPath queries
func (e *ClusterE2ETest) ValidateNetworkUpWithJSONPath() {
	e.T.Log("Validating network using JSONPath queries")

	// Get all nodes using JSONPath to extract external IPs
	jsonPath := "{range .items[*]}{.metadata.name}{':'}{range .status.addresses[?(@.type=='ExternalIP')]}{.address}{','}{end}{'\n'}{end}"

	output, err := e.KubectlClient.ExecuteCommand(context.Background(),
		"get", "nodes",
		"-o", fmt.Sprintf("jsonpath=%s", jsonPath),
		"--kubeconfig", e.KubeconfigFilePath())

	if err != nil {
		e.T.Fatalf("Failed to get node external IPs: %v", err)
	}

	// Parse the output
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		nodeName := parts[0]
		ipsStr := strings.TrimSuffix(parts[1], ",")

		if ipsStr == "" {
			e.T.Fatalf("Node %s has no external IPs", nodeName)
		}

		ips := strings.Split(ipsStr, ",")
		// Remove empty strings
		var validIPs []string
		for _, ip := range ips {
			if strings.TrimSpace(ip) != "" {
				validIPs = append(validIPs, strings.TrimSpace(ip))
			}
		}

		if len(validIPs) < 2 {
			e.T.Fatalf("Node %s does not have 2 external IPs. Found %d IPs: %v",
				nodeName, len(validIPs), validIPs)
		}

		if !e.areIPsDifferent(validIPs) {
			e.T.Fatalf("Node %s has duplicate external IPs: %v", nodeName, validIPs)
		}

		e.T.Logf("Node %s has %d different external IPs: %v ✓",
			nodeName, len(validIPs), validIPs)
	}

	e.T.Log("JSONPath network validation completed successfully")
}

// ValidateNetworkUpWithWaitLoop validates network using WaitJSONPathLoop approach
func (e *ClusterE2ETest) ValidateNetworkUpWithWaitLoop() {
	e.T.Log("Validating network using WaitJSONPathLoop approach")

	// First get all node names
	nodes, err := e.getAllNodes()
	if err != nil {
		e.T.Fatalf("Failed to get nodes: %v", err)
	}

	for _, node := range nodes {
		e.T.Logf("Waiting for node %s to have multiple external IPs", node.Name)

		// Use a custom validation function that checks if we have multiple IPs
		err = e.waitForMultipleExternalIPs(node.Name, "5m")
		if err != nil {
			e.T.Fatalf("Node %s failed to get multiple external IPs within timeout: %v", node.Name, err)
		}

		e.T.Logf("Node %s successfully has multiple external IPs ✓", node.Name)
	}

	e.T.Log("WaitLoop network validation completed successfully")
}

// Helper method to get external IPs from a node
func (e *ClusterE2ETest) getExternalIPsFromNode(node corev1.Node) []string {
	var externalIPs []string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP {
			externalIPs = append(externalIPs, addr.Address)
		}
	}
	return externalIPs
}

// Helper method to check if IPs are different
func (e *ClusterE2ETest) areIPsDifferent(ips []string) bool {
	if len(ips) < 2 {
		return false
	}

	seen := make(map[string]bool)
	for _, ip := range ips {
		if seen[ip] {
			return false // Found duplicate
		}
		seen[ip] = true
	}
	return true
}

// Helper method to wait for multiple external IPs using a custom approach
func (e *ClusterE2ETest) waitForMultipleExternalIPs(nodeName, timeout string) error {
	// Parse timeout
	timeoutDuration, err := time.ParseDuration(timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout format: %v", err)
	}

	deadline := time.Now().Add(timeoutDuration)

	for time.Now().Before(deadline) {
		// Get the specific node
		output, err := e.KubectlClient.ExecuteCommand(context.Background(),
			"get", "node", nodeName,
			"-o", "json",
			"--kubeconfig", e.KubeconfigFilePath())

		if err != nil {
			e.T.Logf("Failed to get node %s, retrying: %v", nodeName, err)
			time.Sleep(10 * time.Second)
			continue
		}

		// Parse the node JSON
		var node corev1.Node
		if err := json.Unmarshal(output.Bytes(), &node); err != nil {
			e.T.Logf("Failed to parse node JSON, retrying: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		// Check external IPs
		externalIPs := e.getExternalIPsFromNode(node)
		if len(externalIPs) >= 2 && e.areIPsDifferent(externalIPs) {
			e.T.Logf("Node %s now has %d different external IPs: %v",
				nodeName, len(externalIPs), externalIPs)
			return nil
		}

		e.T.Logf("Node %s has %d external IPs, waiting for 2+ different IPs: %v",
			nodeName, len(externalIPs), externalIPs)
		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("timeout waiting for node %s to have multiple external IPs", nodeName)
}

// Helper method to get all nodes in the cluster using kubectl
func (e *ClusterE2ETest) getAllNodes() ([]corev1.Node, error) {
	params := []string{"get", "nodes", "-o", "json", "--kubeconfig", e.KubeconfigFilePath()}
	stdOut, err := e.KubectlClient.Execute(context.Background(), params...)
	if err != nil {
		return nil, fmt.Errorf("getting nodes: %v", err)
	}

	response := &corev1.NodeList{}
	err = json.Unmarshal(stdOut.Bytes(), response)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling nodes: %v", err)
	}

	return response.Items, nil
}
